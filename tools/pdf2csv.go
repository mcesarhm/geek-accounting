package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"rsc.io/pdf"
)

type transaction struct {
	Date   string
	Amount float64
	Memo   string
}

func main() {
	substitutionsFile := flag.String("s", "", "substitutions file")
	flag.Parse()

	if len(flag.Args()) < 1 {
		fmt.Fprintln(os.Stderr, "usage: pdf2csv file.pdf [-s substitutions.csv]")
		os.Exit(2)
	}
	r, err := pdf.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	transactions := []transaction{}
	var (
		year string
		date string
		memo string
	)
	start := false
PageLoop:
	for i := 1; i <= r.NumPage(); i++ {
		words := findWords(r.Page(i).Content().Text)
		for _, t := range words {
			if t.X == 207.6 && t.S == "Saldo anterior" {
				start = true
				continue
			}
			if start && t.X == 209.52 && t.S == "Saldo em C/C" {
				break PageLoop
			}
			if start && t.X >= 195 && t.X <= 210 &&
				t.S != "descrição" && t.S != "(-) Saldo a liberar" &&
				t.S != "Saldo final disponivel" {
				if memo != "" {
					fmt.Fprintf(os.Stderr, "Malformed: %v\n", t.S)
					os.Exit(2)
				}
				memo = t.S
			}
			if start && t.X >= 150 && t.X <= 153 && t.S != "data" {
				arr := strings.Split(t.S, "/")
				date = year + "-" + arr[1] + "-" + arr[0]
			}
			if start && (t.X+t.W >= 457 && t.X+t.W <= 461 || t.X+t.W >= 394 && t.X+t.W <= 398) &&
				t.S != "saídas R$" && t.S != "entradas R$" &&
				t.S != "(débitos)" && t.S != "(créditos)" {
				var (
					amount float64
					err    error
				)
				t.S = strings.Replace(t.S, ".", "", -1)
				t.S = strings.Replace(t.S, ",", ".", 1)
				if strings.HasSuffix(t.S, "-") {
					amount, err = strconv.ParseFloat("-"+t.S[0:len(t.S)-1], 64)
				} else {
					amount, err = strconv.ParseFloat(t.S, 64)
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error parsing amount: %v", t.S)
					os.Exit(2)
				}
				transactions = append(transactions, transaction{date, amount, memo})
				memo = ""
			}
			if !start && i == 1 && t.X >= 514 && t.X <= 515 && t.Y >= 763 && t.Y <= 764 {
				year = t.S[4:]
			}
		}
	}
	var substitutions Substitutions
	if *substitutionsFile != "" {
		if substitutions, err = NewSubstitutions(*substitutionsFile); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
	for _, t := range transactions {
		account, amount, memo := substitutions.ReplaceData(t.Amount, t.Memo)
		fmt.Printf("\"%v:%v\",%v,\"%v\"\n", account, amount, t.Date, memo)
	}
}

func findWords(chars []pdf.Text) (words []pdf.Text) {
	// Sort by Y coordinate and normalize.
	const nudge = 1
	sort.Sort(pdf.TextVertical(chars))
	old := -100000.0
	for i, c := range chars {
		if c.Y != old && math.Abs(old-c.Y) < nudge {
			chars[i].Y = old
		} else {
			old = c.Y
		}
	}

	// Sort by Y coordinate, breaking ties with X.
	// This will bring letters in a single word together.
	sort.Sort(pdf.TextVertical(chars))

	// Loop over chars.
	for i := 0; i < len(chars); {
		// Find all chars on line.
		j := i + 1
		for j < len(chars) && chars[j].Y == chars[i].Y {
			j++
		}
		var end float64
		// Split line into words (really, phrases).
		for k := i; k < j; {
			ck := &chars[k]
			s := ck.S
			end = ck.X + ck.W
			charSpace := ck.FontSize / 6
			wordSpace := ck.FontSize * 2 / 3
			l := k + 1
			for l < j {
				// Grow word.
				cl := &chars[l]
				if sameFont(cl.Font, ck.Font) && math.Abs(cl.FontSize-ck.FontSize) < 0.1 && cl.X <= end+charSpace {
					s += cl.S
					end = cl.X + cl.W
					l++
					continue
				}
				// Add space to phrase before next word.
				if sameFont(cl.Font, ck.Font) && math.Abs(cl.FontSize-ck.FontSize) < 0.1 && cl.X <= end+wordSpace {
					s += " " + cl.S
					end = cl.X + cl.W
					l++
					continue
				}
				break
			}
			f := ck.Font
			f = strings.TrimSuffix(f, ",Italic")
			f = strings.TrimSuffix(f, "-Italic")
			words = append(words,
				pdf.Text{Font: f, FontSize: ck.FontSize, X: ck.X, Y: ck.Y, W: end - ck.X, S: s})
			k = l
		}
		i = j
	}

	return words
}

func sameFont(f1, f2 string) bool {
	f1 = strings.TrimSuffix(f1, ",Italic")
	f1 = strings.TrimSuffix(f1, "-Italic")
	f2 = strings.TrimSuffix(f1, ",Italic")
	f2 = strings.TrimSuffix(f1, "-Italic")
	return strings.TrimSuffix(f1, ",Italic") == strings.TrimSuffix(f2, ",Italic") || f1 == "Symbol" || f2 == "Symbol" || f1 == "TimesNewRoman" || f2 == "TimesNewRoman"
}
