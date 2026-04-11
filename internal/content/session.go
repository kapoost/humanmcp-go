package content

import (
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"
)

// Fragmenty polskiej poezji do generowania haseł sesji.
// Źródła: Szymborska, Herbert, Miłosz, Leśmian, Baczyński,
// Broniewski, Grochowiak, Poświatowska, Świrszczyńska,
// Białoszewski, Wojaczek, Stachura, Kaczmarski, Gintrowski, Harasymowicz.
var poetryFragments = []string{
	// Szymborska
	"nic dwa razy się nie zdarza",
	"nienawiść nie zna żartów",
	"wolę kino",
	"pod jedną gwiazdą",
	"koniec i początek",
	"możliwości są tylko dwie",
	"chwila jest i przemija",
	"jestem tym czym jestem",
	"ludzie na moście",
	"widok z ziarnkiem piasku",
	// Herbert
	"ocalałem prowadzony na rzeź",
	"Pan Cogito rozmyśla",
	"kamień jest istotą doskonałą",
	"potęga smaku",
	"co ocaleje z uczty bogów",
	"raport z oblężonego miasta",
	"modlitwa pana cogito",
	"struna światła",
	"rovigo",
	"pan cogito obserwuje",
	// Miłosz
	"nie ma nic na końcu drogi",
	"ziemia ulro",
	"świat jest taki jak mówię",
	"który skrzywdziłeś człowieka prostego",
	"dolina issy",
	"na brzegu rzeki",
	"rodzinna europa",
	"druga przestrzeń",
	"gdy myślę o przyszłości",
	// Leśmian
	"dziewczyna szła przez świat",
	"w malinowym chruśniaku",
	"dusiołek łazi po niebie",
	"topielec zanurzył się w wodzie",
	"w rzeczach jest głębina",
	"las bezlistny i wiosna",
	"w siódmej wodzie za lasem",
	// Baczyński
	"wyrośniesz z tej ziemi",
	"biały wiatr w górach",
	"Historia może zabić",
	"ten czas jak nóż",
	"pokolenie wyrosłe z grobu",
	"niebo złote ci otworzę",
	"elegia o chłopcu polskim",
	// Broniewski
	"nagle ze snu wyrwany",
	"bagnet na broń",
	"ulica Miła",
	"słowo o Stalinie",
	"żołnierz polski",
	// Grochowiak
	"płonąca żyrafa",
	"nie będzie piękniej już",
	"kanon",
	"ikar",
	"menuet z pogrzebaczem",
	// Poświatowska
	"jestem po tej samej stronie",
	"kocham cię bardzo",
	"powiedz mi jak mnie kochasz",
	"wiersz bez sensu",
	"wyznanie",
	// Świrszczyńska
	"budowałam barykadę",
	"jestem kobietą",
	"rozmowa z matką",
	"szczęście",
	"wielkie słowa",
	// Wojaczek
	"jest taka pustka po człowieku",
	"nic nie wiem o śmierci",
	"sezon",
	"anatomia",
	"czarna msza",
	// Stachura
	"uciekaj ze mną na kraj świata",
	"ty mi powiedz ziemio",
	"siekierezada",
	"wszystko jest poezją",
	"wędrówka z motylem",
	"missa pagana",
	"chodzi mi o to aby język giętki",
	// Kaczmarski
	"mury runą runą runą",
	"nasza klasa",
	"obława",
	"raj",
	"epitafium dla Włodka Wysockiego",
	"ballada o spalonej Warszawie",
	"zbroja",
	"autoportret z kamerą",
	"na gruzach Homeru",
	// Gintrowski
	"adagio",
	"tren",
	"modlitwa",
	"pejzaż",
	"wiatr",
	// Harasymowicz
	"cuda czynię nieustannie",
	"barany z gór tatrzańskich",
	"list do zimy",
	"anioł w krakowie",
}

// SessionCode przechowuje aktualne hasło sesji.
type SessionCode struct {
	mu        sync.RWMutex
	code      string
	expiresAt time.Time
	rotateEvery time.Duration
}

// NewSessionCode tworzy nowy generator haseł z automatyczną rotacją.
func NewSessionCode(rotateEvery time.Duration) *SessionCode {
	sc := &SessionCode{rotateEvery: rotateEvery}
	sc.rotate()
	go sc.autoRotate()
	return sc
}

// Current zwraca aktualne hasło i czas wygaśnięcia.
func (sc *SessionCode) Current() (string, time.Time) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.code, sc.expiresAt
}

// Rotate ręcznie rotuje hasło (np. na żądanie z dashboardu).
func (sc *SessionCode) Rotate() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.rotate()
	return sc.code
}

// Verify sprawdza czy podany kod jest aktualny (case-insensitive).
func (sc *SessionCode) Verify(code string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if time.Now().After(sc.expiresAt) {
		return false
	}
	return strings.EqualFold(
		strings.TrimSpace(code),
		strings.TrimSpace(sc.code),
	)
}

func (sc *SessionCode) rotate() {
	sc.code = pickFragment()
	sc.expiresAt = time.Now().Add(sc.rotateEvery)
}

func (sc *SessionCode) autoRotate() {
	for {
		sc.mu.RLock()
		untilExpiry := time.Until(sc.expiresAt)
		sc.mu.RUnlock()

		if untilExpiry <= 0 {
			untilExpiry = time.Minute
		}
		time.Sleep(untilExpiry)

		sc.mu.Lock()
		if time.Now().After(sc.expiresAt) {
			sc.rotate()
		}
		sc.mu.Unlock()
	}
}

func pickFragment() string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(poetryFragments))))
	if err != nil {
		return poetryFragments[0]
	}
	return poetryFragments[n.Int64()]
}
