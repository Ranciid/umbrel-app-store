#!/usr/bin/env python3
"""Apply native Kindle integration to a clean vavallee/Bindery v1.52.0 checkout.

Fail closed if upstream code changes, rather than silently producing an
untested patch against an incompatible version.
"""
from pathlib import Path
import sys

root = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(".").resolve()

def replace_once(relative: str, old: str, new: str) -> None:
    path = root / relative
    content = path.read_text()
    occurrences = content.count(old)
    if occurrences != 1:
        raise SystemExit(f"{relative}: expected anchor exactly once, got {occurrences}: {old[:95]!r}")
    path.write_text(content.replace(old, new, 1))

replace_once(
    "cmd/bindery/main.go",
    '\t// Router\n\tr := chi.NewRouter()',
    '''\t// Native Kindle delivery: one integrated worker and API, no companion container.
\tkindleHandler, err := api.NewKindleHandler(database, bookRepo, settingsRepo)
\tif err != nil {
\t\tslog.Error("kindle delivery initialization failed", "error", err)
\t\tos.Exit(1)
\t}
\tgo kindleHandler.Run(appCtx)

\t// Router
\tr := chi.NewRouter()''',
)
replace_once(
    "cmd/bindery/main.go",
    '''\tr.Route("/api/v1", func(r chi.Router) {
\t\tuseAPIAuth(r, authProvider)
\t\tregisterAPIErrorHandlers(r)
''',
    '''\tr.Route("/api/v1", func(r chi.Router) {
\t\tuseAPIAuth(r, authProvider)
\t\tregisterAPIErrorHandlers(r)
\t\t// Kindle config, history and sending are admin-only; protect the
\t\t// credentials and restrict outgoing file delivery to administrators.
\t\tr.Group(func(r chi.Router) {
\t\t\tr.Use(auth.RequireAdmin)
\t\t\tr.Get("/kindle/config", kindleHandler.Config)
\t\t\tr.Put("/kindle/config", kindleHandler.SaveConfig)
\t\t\tr.Post("/kindle/test", kindleHandler.Test)
\t\t\tr.Post("/kindle/send/{id}", kindleHandler.SendBook)
\t\t\tr.Get("/kindle/history", kindleHandler.History)
\t\t})
''',
)
replace_once(
    "web/src/pages/SettingsPage.tsx",
    "const CalibreTab = lazyWithReload(() => import('./settings/CalibreTab'), './settings/CalibreTab')",
    """const CalibreTab = lazyWithReload(() => import('./settings/CalibreTab'), './settings/CalibreTab')
const KindleTab = lazyWithReload(() => import('./settings/KindleTab'), './settings/KindleTab')""",
)
replace_once(
    "web/src/pages/SettingsPage.tsx",
    "| 'calibre' | 'abs' |",
    "| 'calibre' | 'kindle' | 'abs' |",
)
replace_once(
    "web/src/pages/SettingsPage.tsx",
    "'blocklist', 'calibre', 'abs',",
    "'blocklist', 'calibre', 'kindle', 'abs',",
)
replace_once(
    "web/src/pages/SettingsPage.tsx",
    "tabs: ['notifications', 'calibre', 'abs', 'bookorbit', 'grimmory', 'api-keys']",
    "tabs: ['notifications', 'calibre', 'kindle', 'abs', 'bookorbit', 'grimmory', 'api-keys']",
)
replace_once(
    "web/src/pages/SettingsPage.tsx",
    "case 'api-keys': return t('settings.tabs.apiKeys')",
    "case 'kindle': return 'Kindle'\n      case 'api-keys': return t('settings.tabs.apiKeys')",
)
replace_once(
    "web/src/pages/SettingsPage.tsx",
    "case 'calibre': return <CalibreTab />",
    "case 'calibre': return <CalibreTab />\n      case 'kindle': return <KindleTab />",
)
replace_once(
    "web/src/pages/BookDetailPage.tsx",
    "import CalibreDeliveryChip from '../components/CalibreDeliveryChip'",
    "import CalibreDeliveryChip from '../components/CalibreDeliveryChip'\nimport SendToKindleButton from '../components/SendToKindleButton'",
)
replace_once(
    "web/src/pages/BookDetailPage.tsx",
    "<CalibreDeliveryChip bookId={book.id} />",
    "<CalibreDeliveryChip bookId={book.id} />\n            {isAdmin && book.ebookFilePath && <SendToKindleButton bookId={book.id} />}",
)

# Verify that generated pieces landed before the Docker build.
for relative in [
    "internal/api/kindle.go",
    "web/src/pages/settings/KindleTab.tsx",
    "web/src/components/SendToKindleButton.tsx",
]:
    if not (root / relative).is_file():
        raise SystemExit(f"Missing Kindle integration file {relative}")

print("Applied native Kindle API + settings UI + Send to Kindle book button.")
