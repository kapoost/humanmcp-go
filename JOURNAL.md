# Dziennik projektu humanMCP

Codzienne notatki z sesji deweloperskich.
Format: co zrobiono, co nie działa, kluczowe wnioski.

---

## 12 kwietnia 2026

**Zrobiono:** humanMCP v0.3: shared stores, upload validation, vault tools, agent write usunięty (tylko owner pisze), leave_message truncacja, auth.New z agentToken, testy zielone (Go race + pytest + Playwright 32 e2e). myśloodsiewnia: Honda S2000 zaindeksowana, Cloudflare Tunnel z auto-update, VAULT_TOKEN nowy, repo mysloodsiewnia publiczne z własnym CI, AppleScript fix, testy e2e wzmocnione, end_session workflow.

**Problemy:** MX-5 1999-2001 PDF to skan — wymaga OCR (tesseract). Chrome extension konflikt dwóch kont Google. mellens.net i trull.org nie serwują plików przez curl. POST /ingest/* w myśloodsiewni bez auth (TODO: VAULT_TOKEN check).

**Wnioski:** AppleScript: open i jump to dwa osobne wywołania — nie można ich zagnieżdżać w jednym tell block. trull.org: HEAD 200 nie znaczy że pliki dostępne. command rm w zsh funkcji nie działa — używaj /bin/rm. auth.New musi przyjmować oba tokeny — zawsze sprawdź sygnaturę konstruktora przed patchowaniem. Poziom 2 (AGENT_TOKEN write) był zbędny — mniejsza powierzchnia ataku = lepiej.

---
