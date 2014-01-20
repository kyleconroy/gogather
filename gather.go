package main

import (
	"code.google.com/p/go.net/html"
	"crypto/md5"
	"net/http"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
)

const (
	prefix = "#ctl00_ctl00_ctl00_MainContent_SubContent_SubContent_"
	gathererUrl = "http://gatherer.wizards.com/Pages/Card/Details.aspx?multiverseid=%d"
)

type Deckbox struct {
	Cards []Card `json:"cards"`
}

func (d Deckbox) MultiverseSet() map[int]bool {
	set := map[int]bool{}

	for _, card := range d.Cards {
		for _, edition := range card.Editions {
			set[edition.MultiverseId] = true
		}
	}

	return set
}

func (d* Deckbox) Add(card Card) error {
	if len(card.Editions) == 0 {
		return fmt.Errorf("%s has no editions", card.Name)
	}

	for i, c := range d.Cards {
		if c.Id == card.Id {
			edition := card.Editions[0]

			for _, e := range c.Editions {
				if e.MultiverseId == edition.MultiverseId {
					return nil
				}
			}

			d.Cards[i].Editions = append(card.Editions, edition)
			return nil
		}
	}

	d.Cards = append(d.Cards, card)
	return nil
}


type Card struct {
	Name          string    `json:"name"`
	Id            string    `json:"id"`
	Types         []string  `json:"types"`
	Subtypes      []string  `json:"subtypes"`
	ConvertedCost int       `json:"converted_cost"`
	ManaCost      string    `json:"mana_cost"`
	Special       string    `json:"special"` //'flip', 'double-faced', 'split'
	PartnerCard   string    `json:"partner_card"`
	RulesText     []string  `json:"rules_text"`
	Power         string    `json:"power"`
	Toughness     string    `json:"toughness"`
	Editions      []Edition `json:"editions"`
}

type Edition struct {
	Set          string   `json:"set"`
	Watermark    string   `json:"watermark"`
	Rarity       string   `json:"rarity"`
	Artist       string   `json:"artist"`
	MultiverseId int      `json:"multiverse_id"`
	FlavorText   []string `json:"flavor_text"`
	Number       string   `json:"number"`
}

func (c Card) ImageURl() string {
	return "http://gatherer.wizards.com/Handlers/Image.ashx?multiverseid="
}

func cardName(n *html.Node) string {
	return ""
}

func extractString(n *html.Node, pattern string) string {
	if div, found := Find(n, pattern); found {
		return strings.TrimSpace(Flatten(div))
	} else {
		return ""
	}
}

func manaSymbol(alt string) string {
	switch alt {
	case "0", "1", "2", "3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15":
		return "{" + alt + "}"
	case "Phyrexian":
		return "{P}"
	case "Phyrexian Green":
		return "{G/P}"
	case "Phyrexian Red":
		return "{R/P}"
	case "Phyrexian Blue":
		return "{U/P}"
	case "Phyrexian Black":
		return "{B/P}"
	case "Phyrexian White":
		return "{W/P}"
	case "White or Blue":
		return "{W/U}"
	case "White or Black":
		return "{W/B}"
	case "Blue or Black":
		return "{U/B}"
	case "Blue or Red":
		return "{U/R}"
	case "Black or Red":
		return "{B/R}"
	case "Black or Green":
		return "{B/G}"
	case "Red or Green":
		return "{R/G}"
	case "Red or White":
		return "{R/W}"
	case "Green or White":
		return "{G/W}"
	case "Green or Blue":
		return "{G/U}"
	case "Two or White":
		return "{2/W}"
	case "Two or Blue":
		return "{2/U}"
	case "Two or Black":
		return "{2/B}"
	case "Two or Red":
		return "{2/R}"
	case "Two or Green ":
		return "{2/G}"
	case "Variable Colorless":
		return "{X}"
	case "Snow":
		return "{S}"
	case "Green":
		return "{G}"
	case "Red":
		return "{R}"
	case "Blue":
		return "{U}"
	case "Black":
		return "{B}"
	case "White":
		return "{W}"
	case "Tap":
		return "{T}"
	case "Untap":
		return "{Q}"
	}
	return ""
}

func FlattenWithSymbols(n *html.Node) string {
	text := ""
	if n.Type == html.TextNode {
		text += n.Data
	}

	if n.Type == html.ElementNode && n.Data == "img" {
		text += manaSymbol(Attr(n, "alt"))
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += FlattenWithSymbols(c)
	}

	return text
}

func extractManaCost(n *html.Node) string {
	cost := ""
	for _, a := range FindAll(n, prefix+"manaRow .value img") {
		cost += manaSymbol(Attr(a, "alt"))
	}
	return cost
}

func extractPT(n *html.Node) (string, string) {
	div, found := Find(n, prefix+"ptRow .value")

	if !found {
		return "", ""
	}

	values := strings.Split(strings.TrimSpace(Flatten(div)), "/")

	if len(values) != 2 {
		return "", ""
	}

	return strings.TrimSpace(values[0]), strings.TrimSpace(values[1])
}

func SplitTrimSpace(source, pattern string) []string {
	result := []string{}

	for _, val := range strings.Split(strings.TrimSpace(source), pattern) {
		result = append(result, strings.TrimSpace(val))
	}

	return result
}

func extractTypes(n *html.Node) ([]string, []string) {
	div, found := Find(n, prefix+"typeRow .value")

	if !found {
		return []string{}, []string{}
	}

	ts := strings.Split(strings.ToLower(strings.TrimSpace(Flatten(div))), "—")

	var types []string
	var subtypes []string

	if len(ts) == 2 {
		types = SplitTrimSpace(ts[0], " ")
		subtypes = SplitTrimSpace(ts[1], " ")
	} else {
		types = SplitTrimSpace(ts[0], " ")
		subtypes = []string{}
	}

	return types, subtypes
}

func extractRarity(n *html.Node) string {
	if span, found := Find(n, prefix+"rarityRow .value span"); found {
		return Attr(span, "class")
	} else {
		return ""
	}
}

func extractInt(n *html.Node, pattern string) int {
	div, found := Find(n, pattern)

	if !found {
		return 0
	}

	number, err := strconv.Atoi(strings.TrimSpace(Flatten(div)))

	if err != nil {
		return 0
	}

	return number
}

func extractText(n *html.Node, pattern string) []string {
	rules := []string{}
	for _, node := range FindAll(n, pattern) {
		rules = append(rules, strings.TrimSpace(FlattenWithSymbols(node)))
	}
	return rules
}

func hash(in string) string {
	h := md5.New()
	io.WriteString(h, in)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func ParseCard(page io.Reader, multiverseid int) (Card, error) {
	doc, err := html.Parse(page)

	card := Card{}
	edition := Edition{}

	if err != nil {
		return card, err
	}

	card.Name = extractString(doc, prefix+"nameRow .value")
	card.Id = hash(card.Name)
	card.ManaCost = extractManaCost(doc)
	card.ConvertedCost = extractInt(doc, prefix+"cmcRow .value")
	card.RulesText = extractText(doc, prefix+"textRow .value .cardtextbox")
	card.Types, card.Subtypes = extractTypes(doc)
	card.Power, card.Toughness = extractPT(doc)

	edition.Number = extractString(doc, prefix+"numberRow .value")
	edition.Artist = extractString(doc, prefix+"artistRow .value")
	edition.Set = extractString(doc, prefix+"setRow .value")
	edition.FlavorText = extractText(doc, prefix+"flavorRow .value .cardtextbox")
	edition.Rarity = extractRarity(doc)
	edition.MultiverseId = multiverseid
	edition.Watermark = extractString(doc, prefix+"markRow .value")

	card.Editions = append(card.Editions, edition)

	return card, nil
}

func main() {
	flag.Parse()

	path := flag.Arg(0)

	blob, err := ioutil.ReadFile(path)

	if err != nil {
		log.Println("Couldn't open file, starting from scratch")
		blob = []byte(`{"cards": []}`)
	}

	var box Deckbox

	err = json.Unmarshal(blob, &box)

	if err != nil {
		log.Fatalf("Can't decode a JSON object in %s", path)
	}

	// Create a set of all multiverse ids I've already seen
	set := box.MultiverseSet()

	// Create a Card channel and a int channel
	cardChan := make(chan Card)
	multiverseChan := make(chan int, 10)

	// Start a routines to count ids
	go func() {
		for i := 1; i < 10000000; i++ {
			if !set[i] {
				multiverseChan <- i
			}
		}
	}()

	// Start N go routines to go fetch and parse cards
	for j := 0; j < 10; j++ {
		go func() {
			for {
				id := <-multiverseChan
				log.Printf("Fetch %d", id)

				url := fmt.Sprintf(gathererUrl, id)
				resp, err := http.Get(url)

				if err != nil {
					log.Println(err)
					continue
				}
	
				card, _ := ParseCard(resp.Body, id)

				cardChan <- card
			}
		}()
	}

	// One go rotine pulls cards off the channel, adds them to the database
	// And flushes it to memory
	for {
		card := <-cardChan

		err := box.Add(card)

		if err != nil {
			log.Printf("ERROR Couldn't add card to database %s", card.Name)
			continue
		}

		log.Printf("Added %s", card.Name)

		blob, err := json.Marshal(box)

		if err != nil {
			log.Fatalf("Couldn't marshal card database to JSON: %s", err)
		}

		err = ioutil.WriteFile(path, blob, 0644)

		if err != nil {
			log.Fatalf("Couldn't write card database: %s", err)
		}
	}
}
