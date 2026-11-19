package api

import (
    "testing"
)

func TestKindleRecipientValidation(t *testing.T) {
    cases:=[]struct{ raw string; valid bool }{
        {"reader@kindle.com",true},
        {"A@kindle.com,B@kindle.com",true},
        {"reader@gmail.com",false},
        {"anything@kindle.com\r\nBcc:evil@example.org",false},
        {"",false},
        {"reader@kindle.com, other@example.org",false},
    }
    for _,tt:=range cases {
        v,err:=kindleRecipients(tt.raw)
        if (err==nil)!=tt.valid {t.Errorf("kindleRecipients(%q)=%v,err=%v; valid=%v",tt.raw,v,err,tt.valid)}
    }
}

func TestKindleConfigNeedsTLSAndCredentials(t *testing.T) {
    good:=kindleConfig{
        Host:"mail.smtp2go.com",Port:587,Login:"user",Password:"secret",
        From:"books@example.org",Recipients:"reader@kindle.com",
    }
    if err:=good.validate();err!=nil {t.Fatal(err)}
    wrongPort:=good; wrongPort.Port=25
    if err:=wrongPort.validate();err==nil {t.Fatal("plaintext SMTP must be refused")}
    missingPassword:=good; missingPassword.Password=""
    if err:=missingPassword.validate();err==nil {t.Fatal("missing password must be refused")}
    wrongRecipient:=good; wrongRecipient.Recipients="attacker@example.org"
    if err:=wrongRecipient.validate();err==nil {t.Fatal("non-Kindle recipient must be refused")}
}
