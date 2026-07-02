package compress

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

const defaultFallbackTokens = 2000

var tokenRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

func fallbackDigest(instruction string, raw *envelope.RawContext, maxTokens int) envelope.ContextDigest {
	if maxTokens <= 0 {
		maxTokens = defaultFallbackTokens
	}
	if raw == nil || len(raw.Units) == 0 {
		return envelope.ContextDigest{Summary: "no raw context available"}
	}
	ranked := rankUnits(instruction, raw.Units)
	digest := envelope.ContextDigest{Summary: "deterministic fallback selected highest-overlap raw units"}
	remaining := maxTokens
	for _, item := range ranked {
		if remaining <= 0 || len(digest.Items) == 40 {
			break
		}
		quote := fitQuote(item.unit.Text, remaining)
		if quote == "" {
			continue
		}
		remaining -= llm.Estimate(quote)
		digest.Items = append(digest.Items, envelope.DigestItem{
			UnitID:    item.unit.ID,
			Path:      item.unit.Path,
			Relevance: item.relevance,
			Spans: []envelope.DigestSpan{{
				StartLine: item.unit.StartLine,
				EndLine:   item.unit.EndLine,
				Quote:     quote,
			}},
		})
	}
	return digest
}

func rankUnits(instruction string, units []envelope.RawUnit) []rankedUnit {
	query := tokenCounts(instruction)
	df := documentFrequency(units)
	scores := make([]rankedUnit, 0, len(units))
	maxScore := 0.0
	for _, unit := range units {
		score := scoreUnit(query, tokenCounts(unit.Path+" "+unit.Text), df, len(units))
		if score > maxScore {
			maxScore = score
		}
		scores = append(scores, rankedUnit{unit: unit, score: score})
	}
	for i := range scores {
		if maxScore > 0 {
			scores[i].relevance = int(math.Round(scores[i].score / maxScore * 100))
		}
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].unit.ID < scores[j].unit.ID
		}
		return scores[i].score > scores[j].score
	})
	return scores
}

func scoreUnit(query map[string]int, doc map[string]int, df map[string]int, docs int) float64 {
	score := 0.0
	for term, qtf := range query {
		tf := doc[term]
		if tf == 0 {
			continue
		}
		idf := math.Log(1 + float64(docs+1)/float64(df[term]+1))
		score += float64(qtf*tf) * idf
	}
	return score
}

func documentFrequency(units []envelope.RawUnit) map[string]int {
	df := map[string]int{}
	for _, unit := range units {
		for term := range tokenCounts(unit.Path + " " + unit.Text) {
			df[term]++
		}
	}
	return df
}

func tokenCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, token := range tokenRE.FindAllString(strings.ToLower(text), -1) {
		counts[token]++
	}
	return counts
}

func fitQuote(text string, maxTokens int) string {
	if len([]rune(text)) > 2000 {
		text = string([]rune(text)[:2000])
	}
	if llm.Estimate(text) <= maxTokens {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if llm.Estimate(string(runes[:mid])) <= maxTokens {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo])
}

type rankedUnit struct {
	unit      envelope.RawUnit
	score     float64
	relevance int
}
