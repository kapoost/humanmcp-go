# Plan naprawy i rozbudowy testów

Stan na 2026-10-20. Pisany po dodaniu działu `code`, przy okazji którego wyszło,
że pakiet testów nie kompiluje się od maja.

> **Aktualizacja 2026-11-09.** Etap 0 i Etap 1 wykonane w commicie `908b19c`.
> 17 wywołań `NewSkillStore` poprawione, `go test ./... -race` przechodzi.
> Etap 1 okazał się pusty: po odblokowaniu kompilacji pierwszy przebieg był
> od razu zielony, więc przez te miesiące nic nie zgniło — zepsuty był
> wyłącznie build. Etap 2 (testy działu `code`) i Etap 3 (bramka, żeby nie
> zgniło znowu) **nadal otwarte** — w tym repo CI istnieje, ale deploy idzie
> ręcznym `fly deploy`, który bramkę omija.

---

## Stan wyjściowy — fakty, nie wrażenia

**Testy nie kompilują się od 2026-05-16** (commit `08bac36` „skills: SkillStore
vault-fetch + bootstrap downgrade transparency"). Ten commit zmienił sygnaturę na
`NewSkillStore(contentDir, vaultURL string)`, ale nie zaktualizował wywołań w testach.

Jedna przyczyna, 17 wywołań, trzy pakiety:

| plik | wywołań |
|---|---|
| `internal/content/skill_test.go` | 11 |
| `internal/mcp/handler_test.go` | 5 |
| `internal/web/handler_test.go` | 1 |

To **cały** zbiór błędów — sprawdzone przez `go test -c -gcflags=-e` (bez ucinania
na dziesiątym błędzie). Nie ma drugiej, ukrytej warstwy problemów.

**`main` jest zepsuty tak samo.** `git show main:internal/content/skill_test.go`
pokazuje 11 jednoargumentowych wywołań przy dwuargumentowej sygnaturze.

**Konsekwencja, która jeszcze nie wybuchła:** `.github/workflows/deploy.yml` ma
`test-go` z `go test ./... -race`, a job `deploy` ma `needs: [test-go]`. Czyli
automatyczny deploy jest martwy od maja. Produkcja żyje, bo deployujesz ręcznie
(`fly deploy` z `~/.zshrc`), omijając bramkę. Następny merge do `main` tego nie ominie.

**Co to znaczy naprawdę:** `go build ./...` przechodzi, więc kod produkcyjny jest
kompilowalny. Ale pakiety `content`, `mcp` i `web` są bez żadnego pokrycia od pięciu
miesięcy — a w tym czasie weszły m.in. reguły w `instructions`, token `ack`
bramkujący prywatne narzędzia i vault-fetch w SkillStore. Żadna z tych rzeczy
nie została przetestowana przez istniejące 4400 linii testów.

---

## Etap 0 — odblokowanie kompilacji

Mechaniczne. `vaultURL == ""` to udokumentowane zachowanie („When empty, behaves as
pure dir-based store", `internal/content/skill.go:97`), więc testom wystarczy pusty
drugi argument — nie trzeba stawiać atrapy vaulta.

```
grep -rln 'NewSkillStore(' internal/ | grep _test
# w każdym: NewSkillStore(x)  →  NewSkillStore(x, "")
```

**Kryterium wyjścia:** `go test ./... -race -timeout 60s` kompiluje się i kończy
*jakimkolwiek* wynikiem. Zielony nie jest tu celem — celem jest zobaczyć, co pada.

**Uwaga:** nie poprawiać przy okazji padających asercji. Etap 0 kończy się na
kompilacji; naprawianie treści testów w tym samym kroku zatrze granicę między
„test był zepsuty" a „kod się zmienił".

---

## Etap 1 — inwentaryzacja tego, co zgniło

Dopiero po Etapie 0 wiadomo, ile z 4400 linii testów jeszcze opisuje rzeczywistość.
Każdy padający test rozstrzygnąć w jedną z trzech kategorii i **zapisać którą**:

1. **Test ma rację, kod się zepsuł** → bug, naprawiać kod.
2. **Kod zmienił się celowo, test opisuje stary świat** → zaktualizować test,
   w commicie napisać *dlaczego* zachowanie się zmieniło.
3. **Test testował coś, czego już nie ma** → usunąć, nie komentować.

Szczególnie podejrzane, bo zmieniane po maju bez osłony testów:
- `bootstrap_session` — degradacja do `minimal` przy niedostępnym vaulcie
- token `ack` i bramkowanie prywatnych narzędzi (`72a0a86`)
- `hardRules` doklejane na początek `instructions` (`6fc5722`)

---

## Etap 2 — testy działu `code`

Dziś dział jest zweryfikowany ręcznie (uruchomiony serwer, `curl`, odpowiedzi MCP),
ale nie ma ani jednego testu automatycznego. Konkretne przypadki do dopisania:

### `internal/content/content_test.go`
- `TestLoadCodePieces` — pliki z `content/code/` wczytują się z `Type == "code"`,
  `URL` niepuste, `Published` sparsowane z `YYYY-MM-DD`.
- `TestCodePiecesSortedByPushDate` — `List()` zwraca je malejąco; to na tym opiera
  się obietnica „newest push first" z `instructions`.

### `internal/mcp/handler_test.go`
Obok istniejącego `TestListContentFilterByType`:
- `TestListContentCodeIncludesURL` — **najważniejszy**. Pierwsza wersja działu
  przeszła wszystkie ręczne sprawdzenia i i tak gubiła `url` w wyniku, bo formatter
  go nie drukował. Agent dostawał nazwy repo bez linków. Ten test pilnuje dokładnie
  tej regresji.
- `TestListContentCodeUsesLastPushLabel` — `code` ma `last push:`, pozostałe typy
  dalej `date:`.
- `TestInitializeMentionsCode` — `instructions` zawiera wskazówkę o
  `list_content(type="code")`. Bez tego dział istnieje, ale żaden agent się o nim
  nie dowie — a to był cały cel.

### `internal/web/handler_test.go`
Obok istniejącego `TestIndexRenders`:
- `TestIndexRendersCodeSection` — `id="kod"` w HTML, link `github.com/kapoost/...`.
- `TestCodePieceNotInPoemsSection` — kawałek `code` nie wycieka do `#wiersze`.
- `TestCodeSurvivesPersonaFilter` — regresja na wyrost: `isPersonaPiece` dopasowuje
  przez `strings.Contains` na slugu, więc persona o krótkim slugu połknęłaby repo
  o podobnej nazwie. Dlatego `code` jest sprawdzany *przed* tym filtrem
  (`handler_web.go`). Test ma to przypiąć, bo inaczej ktoś to „uprości" z powrotem.

---

## Etap 3 — żeby nie zgniło znowu

Bramka już istnieje i jest poprawnie napisana — problem jest taki, że da się ją obejść,
i przez pięć miesięcy była obchodzona.

- **Przyczyna źródłowa to ręczny deploy, nie brak CI.** Dopóki `fly deploy` z lokalnej
  maszyny jest normalną drogą na produkcję, czerwone CI nic nie blokuje. Albo deploy
  idzie przez `main`, albo lokalna funkcja `deploy` w `~/.zshrc` najpierw odpala
  `go test ./...` i przerywa przy niezerowym wyjściu.
- **Uruchamiać CI też na gałęziach.** Dziś `on: push: branches: [main]` — robota na
  `mcp/rules-in-instructions` nie jest sprawdzana aż do PR-a. Dodanie `branches: ['**']`
  dla samego `test-go` (bez `deploy`) daje sygnał od razu.
- **`package.json` ma `"test": "echo Error: no test specified && exit 1"`**, mimo że
  `e2e/keyboard.spec.ts` i `playwright.config.ts` istnieją. Albo podpiąć Playwright,
  albo usunąć atrapę — dziś wygląda, jakby testów JS nie było wcale.

---

## Poza zakresem tego planu

- Przepisywanie istniejących 4400 linii testów na inną konwencję.
- Pokrycie `internal/auth`, `internal/config`, `internal/oauth`, `cmd/*` — dziś
  `[no test files]`, ale to osobna decyzja, nie skutek majowej regresji.
- Testy e2e Playwright — `e2e/keyboard.spec.ts` istnieje i nie był ruszany.

---

## Weryfikacja na koniec

```
go build ./...                      # przechodzi już dziś
go test ./... -race -timeout 60s    # cel: zielone
go vet ./...                        # dziś: błędy kompilacji testów
```

Nie uznawać za zrobione na podstawie samego `go build` — to właśnie dlatego
zepsucie przeżyło pięć miesięcy. `build` przechodził przez cały ten czas.
