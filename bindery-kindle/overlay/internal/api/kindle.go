// Native Send to Kindle integration for the Ranciid Bindery build.
// SMTP acceptance is NOT the same as successful Amazon/Kindle delivery.
package api

import (
    "context"
    "crypto/tls"
    "database/sql"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log/slog"
    "mime"
    "net"
    "net/mail"
    "net/http"
    "net/smtp"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/vavallee/bindery/internal/db"
)

const (
    kindleMaxFileSize = 24 * 1024 * 1024
    kindleLogLimit = 50
)

type KindleHandler struct {
    database *sql.DB
    books *db.BookRepo
    settings *db.SettingsRepo
    mu sync.Mutex
}

type kindleConfig struct {
    Host string `json:"host"`
    Port int `json:"port"`
    Login string `json:"login"`
    Password string `json:"password,omitempty"`
    PasswordSet bool `json:"passwordSet"`
    From string `json:"from"`
    Recipients string `json:"recipients"`
    Auto bool `json:"auto"`
}

type kindleSend struct {
    FileID int64
    BookID int64
    Path string
    Title string
}

func NewKindleHandler(database *sql.DB, books *db.BookRepo, settings *db.SettingsRepo) (*KindleHandler, error) {
    if database == nil || books == nil || settings == nil {
        return nil, errors.New("kindle integration requires a database, book repo, and settings repo")
    }
    _, err := database.Exec(`
CREATE TABLE IF NOT EXISTS kindle_sends (
    file_id INTEGER NOT NULL,
    recipient TEXT NOT NULL,
    book_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (file_id, recipient)
);
`)
    if err != nil { return nil, fmt.Errorf("initialize kindle deliveries: %w", err) }
    return &KindleHandler{database:database, books:books, settings:settings}, nil
}

func (h *KindleHandler) setting(ctx context.Context, key, fallback string) string {
    s, err := h.settings.Get(ctx, key)
    if err != nil { slog.Warn("kindle: setting read failed", "key",key,"error",err); return fallback }
    if s == nil { return fallback }
    return s.Value
}

func (h *KindleHandler) config(ctx context.Context) kindleConfig {
    port,_ := strconv.Atoi(h.setting(ctx,"kindle.smtp_port","587"))
    if port == 0 { port = 587 }
    password := h.setting(ctx,"kindle.smtp_password","")
    return kindleConfig{
        Host:h.setting(ctx,"kindle.smtp_host","mail.smtp2go.com"),
        Port:port,
        Login:h.setting(ctx,"kindle.smtp_login",""),
        Password:password,
        PasswordSet:password!="",
        From:h.setting(ctx,"kindle.from",""),
        Recipients:h.setting(ctx,"kindle.recipients",""),
        Auto:h.setting(ctx,"kindle.auto_enabled","false")=="true",
    }
}

func validEmail(value string) (string,error) {
    a,e:=mail.ParseAddress(strings.TrimSpace(value))
    if e!=nil || a==nil || strings.TrimSpace(value)!=a.Address {
        return "",fmt.Errorf("invalid email address")
    }
    return a.Address,nil
}
func kindleRecipients(raw string) ([]string,error) {
    parts:=strings.Split(raw,",")
    if len(parts)==0 || len(parts)>8 { return nil,errors.New("provide 1 to 8 Kindle addresses") }
    out:=make([]string,0,len(parts))
    seen:=map[string]bool{}
    for _,part:=range parts {
        addr,e:=validEmail(part)
        if e!=nil || !strings.EqualFold(strings.Split(addr,"@")[1],"kindle.com") {
            return nil,errors.New("recipients must be valid @kindle.com addresses")
        }
        addr=strings.ToLower(addr)
        if !seen[addr] { out=append(out,addr); seen[addr]=true }
    }
    return out,nil
}

func (c kindleConfig) validate() error {
    host:=strings.TrimSpace(c.Host)
    if host=="" || strings.ContainsAny(host,"/:@ \r\n\t") { return errors.New("invalid SMTP hostname") }
    if c.Port!=465 && c.Port!=587 { return errors.New("use port 587 (STARTTLS) or 465 (TLS)") }
    if c.Login=="" || c.Password=="" { return errors.New("SMTP username and password are required") }
    if _,e:=validEmail(c.From);e!=nil {return errors.New("valid verified sender address required")}
    if _,e:=kindleRecipients(c.Recipients);e!=nil{return e}
    return nil
}

func (h *KindleHandler) Config(w http.ResponseWriter,r *http.Request) {
    c:=h.config(r.Context())
    c.Password=""
    w.Header().Set("Cache-Control","no-store")
    w.Header().Set("Content-Type","application/json")
    _=json.NewEncoder(w).Encode(c)
}

func (h *KindleHandler) SaveConfig(w http.ResponseWriter,r *http.Request) {
    var in kindleConfig
    dec:=json.NewDecoder(io.LimitReader(r.Body,16384))
    dec.DisallowUnknownFields()
    if err:=dec.Decode(&in);err!=nil {
        http.Error(w,"invalid settings payload",http.StatusBadRequest);return
    }
    if in.Port!=465 && in.Port!=587 {
        http.Error(w,"SMTP port must be 587 or 465",http.StatusBadRequest);return
    }
    if in.From!="" { if _,err:=validEmail(in.From);err!=nil {http.Error(w,"invalid sender email",400);return} }
    if in.Recipients!="" { if _,err:=kindleRecipients(in.Recipients);err!=nil {http.Error(w,err.Error(),400);return} }
    if strings.TrimSpace(in.Host)=="" || strings.ContainsAny(in.Host,"/:@ \r\n\t") {
        http.Error(w,"invalid SMTP host",400);return
    }
    if len(in.Password)>512 || len(in.Login)>256 || len(in.From)>256 || len(in.Recipients)>1024 {
        http.Error(w,"setting too long",400);return
    }

    h.mu.Lock()
    defer h.mu.Unlock()
    old:=h.config(r.Context())
    if in.Password=="" {in.Password=old.Password}
    if in.Auto {
        if err:=in.validate();err!=nil {http.Error(w,err.Error(),400);return}
    }
    pairs:=[]db.SettingKV{
        {Key:"kindle.smtp_host",Value:in.Host},
        {Key:"kindle.smtp_port",Value:strconv.Itoa(in.Port)},
        {Key:"kindle.smtp_login",Value:in.Login},
        {Key:"kindle.from",Value:in.From},
        {Key:"kindle.recipients",Value:in.Recipients},
        {Key:"kindle.auto_enabled",Value:strconv.FormatBool(in.Auto)},
    }
    if in.Password!="" {pairs=append(pairs,db.SettingKV{Key:"kindle.smtp_password",Value:in.Password})}
    if in.Auto && !old.Auto {
        var maxID int64
        if err:=h.database.QueryRowContext(r.Context(),"SELECT COALESCE(MAX(id),0) FROM book_files").Scan(&maxID);err!=nil {
            http.Error(w,"unable to initialize automatic delivery",500);return
        }
        pairs=append(pairs,db.SettingKV{Key:"kindle.auto_after_file_id",Value:strconv.FormatInt(maxID,10)})
    }
    if err:=h.settings.SetMany(r.Context(),pairs);err!=nil {
        slog.Error("kindle: save failed","error",err)
        http.Error(w,"save failed",500);return
    }
    w.Header().Set("Content-Type","application/json")
    _,_=io.WriteString(w,`{"ok":true}`)
}

func (h *KindleHandler) Test(w http.ResponseWriter,r *http.Request) {
    c:=h.config(r.Context())
    if err:=c.validate();err!=nil {http.Error(w,err.Error(),400);return}
    // Test to the verified sender inbox, not to Kindle (which rejects mail without a document).
    if err:=sendKindleSMTP(c, c.From, "Bindery SMTP connection test", nil, "SMTP connection accepted. No ebook attached.");err!=nil {
        slog.Warn("kindle: smtp test failed","error",err)
        http.Error(w,"SMTP test failed: "+err.Error(),502);return
    }
    w.Header().Set("Content-Type","application/json")
    _,_=io.WriteString(w,`{"message":"SMTP server accepted the test email; check the sender inbox."}`)
}

func (h *KindleHandler) sendRow(ctx context.Context,file kindleSend,recipient string,force bool) error {
    // Claim first. A crashed process may leave a 'sending' row deliberately:
    // a blind retry after SMTP accepted it could email the same book twice.
    now:=time.Now().UTC().Format(time.RFC3339)
    var res sql.Result
    var err error
    if force {
        res,err=h.database.ExecContext(ctx,`
INSERT INTO kindle_sends(file_id,recipient,book_id,title,status,error,updated_at)
VALUES(?,?,?,?,'sending','',?)
ON CONFLICT(file_id,recipient) DO UPDATE SET status='sending',error='',updated_at=excluded.updated_at`,
            file.FileID,recipient,file.BookID,file.Title,now)
    }else{
        res,err=h.database.ExecContext(ctx,`
INSERT OR IGNORE INTO kindle_sends(file_id,recipient,book_id,title,status,error,updated_at)
VALUES(?,?,?,?,'sending','',?)`,
            file.FileID,recipient,file.BookID,file.Title,now)
    }
    if err!=nil {return fmt.Errorf("record send: %w",err)}
    n,_:=res.RowsAffected()
    if n==0 {return errKindleAlreadyAttempted}

    c:=h.config(ctx)
    if err:=c.validate();err!=nil {
        h.finish(ctx,file.FileID,recipient,"failed",err.Error())
        return err
    }
    info,err:=os.Lstat(file.Path)
    if err!=nil || !info.Mode().IsRegular() {
        h.finish(ctx,file.FileID,recipient,"failed","book file is missing or not a regular file")
        return errors.New("book file is missing or not a regular file")
    }
    ext:=strings.ToLower(filepath.Ext(file.Path))
    if ext!=".epub" && ext!=".pdf" {
        h.finish(ctx,file.FileID,recipient,"failed","only EPUB and PDF can be sent")
        return errors.New("only EPUB and PDF are supported by this sender")
    }
    if info.Size()<1 || info.Size()>kindleMaxFileSize {
        h.finish(ctx,file.FileID,recipient,"failed","book is empty or exceeds 24 MiB attachment limit")
        return errors.New("book is empty or exceeds 24 MiB")
    }
    f,err:=os.Open(file.Path)
    if err!=nil {
        h.finish(ctx,file.FileID,recipient,"failed","cannot open book file")
        return errors.New("cannot open book file")
    }
    body,err:=io.ReadAll(io.LimitReader(f,kindleMaxFileSize+1))
    _=f.Close()
    if err!=nil || len(body)==0 || len(body)>kindleMaxFileSize {
        h.finish(ctx,file.FileID,recipient,"failed","cannot read valid ebook attachment")
        return errors.New("cannot read valid ebook attachment")
    }
    err=sendKindleSMTP(c,recipient,file.Title,body,ext)
    if err!=nil {
        h.finish(ctx,file.FileID,recipient,"failed",err.Error())
        return err
    }
    h.finish(ctx,file.FileID,recipient,"smtp_accepted","")
    slog.Info("kindle: SMTP accepted ebook","bookID",file.BookID,"fileID",file.FileID,"recipient",recipient)
    return nil
}

var errKindleAlreadyAttempted=errors.New("already attempted for this Kindle address; explicit resend is required")
func (h *KindleHandler) finish(ctx context.Context,id int64,recipient,status,msg string) {
    if len(msg)>350 {msg=msg[:350]}
    if _,err:=h.database.ExecContext(ctx,
        "UPDATE kindle_sends SET status=?,error=?,updated_at=? WHERE file_id=? AND recipient=?",
        status,msg,time.Now().UTC().Format(time.RFC3339),id,recipient);err!=nil {
        slog.Error("kindle: could not record send result","error",err)
    }
}
func (h *KindleHandler) SendBook(w http.ResponseWriter,r *http.Request) {
    id,e:=strconv.ParseInt(chi.URLParam(r,"id"),10,64)
    if e!=nil || id<=0 {http.Error(w,"invalid book id",400);return}
    c:=h.config(r.Context())
    if e:=c.validate();e!=nil {http.Error(w,e.Error(),400);return}
    b,e:=h.books.GetByID(r.Context(),id)
    if e!=nil {http.Error(w,"cannot load book",500);return}
    if b==nil {http.NotFound(w,r);return}
    if b.EbookFilePath=="" {http.Error(w,"book has no tracked ebook file",409);return}
    var file kindleSend
    file.BookID=b.ID;file.Title=b.Title;file.Path=b.EbookFilePath
    if e:=h.database.QueryRowContext(r.Context(),
        "SELECT id FROM book_files WHERE book_id=? AND format='ebook' AND path=?",
        b.ID,b.EbookFilePath).Scan(&file.FileID);e!=nil {
        http.Error(w,"ebook file is not tracked in Bindery",409);return
    }
    recipients,e:=kindleRecipients(c.Recipients)
    if e!=nil {http.Error(w,e.Error(),400);return}
    force:=r.URL.Query().Get("force")=="1"
    for _,recipient:=range recipients {
        if e:=h.sendRow(r.Context(),file,recipient,force);e!=nil {
            if errors.Is(e,errKindleAlreadyAttempted) {
                http.Error(w,"Already attempted. Confirm an explicit resend to avoid duplicates.",409)
            }else{
                slog.Warn("kindle: manual delivery failed","bookID",b.ID,"error",e)
                http.Error(w,"Kindle SMTP send failed: "+e.Error(),502)
            }
            return
        }
    }
    w.Header().Set("Content-Type","application/json")
    _,_=io.WriteString(w,`{"status":"smtp_accepted","message":"SMTP accepted the document; Amazon processing and device sync are not guaranteed."}`)
}

func (h *KindleHandler) History(w http.ResponseWriter,r *http.Request) {
    rows,e:=h.database.QueryContext(r.Context(),
        "SELECT book_id,title,recipient,status,error,updated_at FROM kindle_sends ORDER BY updated_at DESC LIMIT ?",kindleLogLimit)
    if e!=nil {http.Error(w,"history unavailable",500);return}
    defer rows.Close()
    result:=make([]map[string]any,0)
    for rows.Next(){
        var id int64
        var title,recipient,status,errorText,updated string
        if rows.Scan(&id,&title,&recipient,&status,&errorText,&updated)!=nil {continue}
        result=append(result,map[string]any{
            "bookId":id,"title":title,"recipient":recipient,
            "status":status,"error":errorText,"updatedAt":updated,
        })
    }
    w.Header().Set("Content-Type","application/json")
    _=json.NewEncoder(w).Encode(result)
}

func (h *KindleHandler) Run(ctx context.Context) {
    ticker:=time.NewTicker(60*time.Second)
    defer ticker.Stop()
    for {
        h.poll(ctx)
        select {
            case <-ctx.Done(): return
            case <-ticker.C:
        }
    }
}
func (h *KindleHandler) poll(ctx context.Context) {
    c:=h.config(ctx)
    if !c.Auto {return}
    if h.setting(ctx,"import.mode","auto")=="external" {
        // External mode is a hand-off to Calibre/CWA; never auto-send its
        // imported catalogue back to Kindle and accidentally duplicate it.
        return
    }
    if e:=c.validate();e!=nil {slog.Warn("kindle: auto-send not configured","error",e);return}
    baseline,e:=strconv.ParseInt(h.setting(ctx,"kindle.auto_after_file_id",""),10,64)
    if e!=nil {
        var latest int64
        if h.database.QueryRowContext(ctx,"SELECT COALESCE(MAX(id),0) FROM book_files").Scan(&latest)==nil {
            _=h.settings.Set(ctx,"kindle.auto_after_file_id",strconv.FormatInt(latest,10))
        }
        return
    }
    rows,e:=h.database.QueryContext(ctx,`
SELECT bf.id,bf.book_id,bf.path,b.title
FROM book_files bf JOIN books b ON b.id=bf.book_id
WHERE bf.format='ebook' AND bf.id>? AND b.excluded=0
ORDER BY bf.id ASC LIMIT 100`,baseline)
    if e!=nil {slog.Warn("kindle: poll query failed","error",e);return}
    files:=make([]kindleSend,0)
    for rows.Next() {
        var file kindleSend
        if rows.Scan(&file.FileID,&file.BookID,&file.Path,&file.Title)==nil {
            files=append(files,file)
        }
    }
    _=rows.Close()
    recipients,e:=kindleRecipients(c.Recipients)
    if e!=nil{return}
    var lastScanned int64
    for _,file:=range files {
        select {case <-ctx.Done(): return;default:}
        lastScanned=file.FileID
        ext:=strings.ToLower(filepath.Ext(file.Path))
        if ext!=".epub"&&ext!=".pdf"{continue}
        for _,recipient:=range recipients {
            // One durable send attempt per file + recipient. No automatic
            // retries on SMTP ambiguity, even after restarting Bindery.
            if e:=h.sendRow(ctx,file,recipient,false);e!=nil && !errors.Is(e,errKindleAlreadyAttempted) {
                slog.Warn("kindle: auto-send failed","bookID",file.BookID,"error",e)
            }
        }
    }
    if lastScanned>0 {
        if e:=h.settings.Set(ctx,"kindle.auto_after_file_id",strconv.FormatInt(lastScanned,10));e!=nil {
            slog.Warn("kindle: failed to update delivery watermark","error",e)
        }
    }
}

func sendKindleSMTP(c kindleConfig,to,subject string,attachment []byte,ext string) error {
    if err:=c.validate();err!=nil{return err}
    from,e:=validEmail(c.From);if e!=nil{return e}
    to,e=validEmail(to);if e!=nil{return e}
    if strings.ContainsAny(subject,"\r\n") {subject=strings.ReplaceAll(strings.ReplaceAll(subject,"\r"," "), "\n"," ")}
    filename:="Book"+ext
    subj:=mime.QEncoding.Encode("utf-8",subject)
    // A MIME attachment is never written to disk. The source book stays in
    // its Bindery library, and the SMTP credentials never enter the message.
    boundary:="bindery-kindle-ebook"
    var message strings.Builder
    message.WriteString("From: "+from+"\r\nTo: "+to+"\r\nSubject: "+subj+"\r\n")
    message.WriteString("MIME-Version: 1.0\r\n")
    if attachment==nil {
        message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n"+ext+"\r\n")
    }else{
        message.WriteString("Content-Type: multipart/mixed; boundary=\""+boundary+"\"\r\n\r\n")
        message.WriteString("--"+boundary+"\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
        message.WriteString("Sent by Bindery.\r\n--"+boundary+"\r\n")
        contentType:="application/epub+zip"
        if ext==".pdf"{contentType="application/pdf"}
        message.WriteString("Content-Type: "+contentType+"; name=\""+filename+"\"\r\n")
        message.WriteString("Content-Disposition: attachment; filename=\""+filename+"\"\r\n")
        message.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
        encoded:=base64.StdEncoding.EncodeToString(attachment)
        for i:=0;i<len(encoded);i+=76 {
            end:=i+76;if end>len(encoded){end=len(encoded)}
            message.WriteString(encoded[i:end]+"\r\n")
        }
        message.WriteString("--"+boundary+"--\r\n")
    }
    // TLS is mandatory. We never fall back to cleartext credentials.
    host:=strings.TrimSpace(c.Host)
    address:=net.JoinHostPort(host,strconv.Itoa(c.Port))
    dialer:=&net.Dialer{Timeout:20*time.Second}
    var conn net.Conn
    if c.Port==465 {
        conn,e=tls.DialWithDialer(dialer,"tcp",address,&tls.Config{ServerName:host,MinVersion:tls.VersionTLS12})
    }else {
        conn,e=dialer.Dial("tcp",address)
    }
    if e!=nil{return fmt.Errorf("SMTP connection: %w",e)}
    defer conn.Close()
    _=conn.SetDeadline(time.Now().Add(90*time.Second))
    client,e:=smtp.NewClient(conn,host)
    if e!=nil{return fmt.Errorf("SMTP greeting: %w",e)}
    defer client.Close()
    if c.Port==587 {
        if ok,_:=client.Extension("STARTTLS");!ok {return errors.New("SMTP server does not support mandatory STARTTLS")}
        if e:=client.StartTLS(&tls.Config{ServerName:host,MinVersion:tls.VersionTLS12});e!=nil{return fmt.Errorf("STARTTLS: %w",e)}
    }
    if ok,_:=client.Extension("AUTH");!ok{return errors.New("SMTP server does not advertise authentication")}
    if e:=client.Auth(smtp.PlainAuth("",c.Login,c.Password,host));e!=nil{return fmt.Errorf("SMTP authentication: %w",e)}
    if e:=client.Mail(from);e!=nil{return fmt.Errorf("SMTP sender refused: %w",e)}
    if e:=client.Rcpt(to);e!=nil{return fmt.Errorf("SMTP recipient refused: %w",e)}
    writer,e:=client.Data();if e!=nil{return fmt.Errorf("SMTP DATA: %w",e)}
    _,e=io.WriteString(writer,message.String())
    if e!=nil {_=writer.Close();return fmt.Errorf("SMTP write: %w",e)}
    if e:=writer.Close();e!=nil{return fmt.Errorf("SMTP acceptance: %w",e)}
    _=client.Quit()
    return nil
}

