package model

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Related notes are worked out from the published model only: the words of
// listed notes, their tags and the notes they link to or are linked from.
// No outside service is asked, and the same notes always give the same
// list (invariant 4): features are numbered in sorted order and scores are
// summed in that order, so floating point results don't depend on map
// iteration.
const (
	relatedMax       = 5    // related notes per note
	relatedMinScore  = 0.08 // below this, notes have too little in common
	relatedTermsKept = 64   // the most telling words of a note; the rest add little
	relatedMinWord   = 3    // shorter words are mostly grammar

	// Each kind of evidence is a cosine similarity between 0 and 1; the
	// score is their weighted sum. Tags are chosen by the author, so they
	// count almost as much as all the words together.
	weightWords = 0.40
	weightTags  = 0.35
	weightLinks = 0.25
)

type feature struct {
	id     int
	weight float64
}

// makeRelated fills Related on every listed note. Unlisted notes get none
// and are never related to another note, like backlinks (§6). The home note
// is never suggested. Notes that already link to each other are left out:
// the reader sees those in the text and among the backlinks.
func (b *builder) makeRelated(s *Site) {
	var notes []*Note
	for _, n := range s.Notes {
		if !n.Unlisted {
			notes = append(notes, n)
		}
	}
	if len(notes) < 2 {
		return
	}

	// Raw features per note, by kind. Keys are prefixed so a tag and a word
	// with the same spelling stay apart.
	words := make([]map[string]float64, len(notes))
	tags := make([]map[string]float64, len(notes))
	links := make([]map[string]float64, len(notes))
	for i, n := range notes {
		words[i] = termCounts(n)
		tags[i] = map[string]float64{}
		for _, t := range n.Tags {
			for x := t; x != nil; x = x.Parent {
				tags[i]["t:"+strings.ToLower(x.Name)] = 1
			}
		}
		links[i] = map[string]float64{}
		for _, l := range n.Links {
			if !l.Target.Unlisted {
				links[i]["l:"+l.TargetID] = 1
			}
		}
		for _, bl := range n.Backlinks { // never from unlisted notes
			links[i]["l:"+bl.SourceID] = 1
		}
	}

	// Words found in too many notes say little about any of them, and make
	// the comparison slow; words in only one note can't relate two.
	maxDF := max(3, len(notes)/4)
	weigh(words, 2, maxDF, relatedTermsKept, true)
	weigh(tags, 2, len(notes), 0, false)
	weigh(links, 2, len(notes), 0, false)

	// Number every feature in sorted order and build each note's vector,
	// each kind scaled so that a dot product gives weight × cosine.
	ids := map[string]int{}
	var names []string
	for _, kind := range [][]map[string]float64{words, tags, links} {
		for _, m := range kind {
			for k := range m {
				if _, ok := ids[k]; !ok {
					ids[k] = 0
					names = append(names, k)
				}
			}
		}
	}
	sort.Strings(names)
	for i, k := range names {
		ids[k] = i
	}
	vecs := make([][]feature, len(notes))
	postings := make([][]feature, len(names)) // feature → (note index, weight), by note index
	for i := range notes {
		var v []feature
		v = appendScaled(v, words[i], ids, weightWords)
		v = appendScaled(v, tags[i], ids, weightTags)
		v = appendScaled(v, links[i], ids, weightLinks)
		sort.Slice(v, func(a, b int) bool { return v[a].id < v[b].id })
		vecs[i] = v
		for _, f := range v {
			postings[f.id] = append(postings[f.id], feature{id: i, weight: f.weight})
		}
	}

	type candidate struct {
		note  *Note
		score float64
	}
	scores := make([]float64, len(notes))
	for i, n := range notes {
		clear(scores)
		for _, f := range vecs[i] {
			for _, p := range postings[f.id] {
				scores[p.id] += f.weight * p.weight
			}
		}
		linked := map[*Note]bool{}
		for _, l := range n.Links {
			linked[l.Target] = true
		}
		for _, bl := range n.Backlinks {
			linked[bl.Source] = true
		}
		var cs []candidate
		for j, m := range notes {
			if j == i || m == s.Home || linked[m] || scores[j] < relatedMinScore {
				continue
			}
			cs = append(cs, candidate{m, scores[j]})
		}
		sort.Slice(cs, func(a, b int) bool {
			if cs[a].score != cs[b].score {
				return cs[a].score > cs[b].score
			}
			return cs[a].note.URL < cs[b].note.URL
		})
		if len(cs) > relatedMax {
			cs = cs[:relatedMax]
		}
		for _, c := range cs {
			n.Related = append(n.Related, c.note)
			n.RelatedIDs = append(n.RelatedIDs, c.note.ID)
		}
	}
}

// termCounts counts a note's words, lowercased, with the title counted
// twice: it is what the note is about.
func termCounts(n *Note) map[string]float64 {
	m := map[string]float64{}
	add := func(text string, times float64) {
		for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			if utf8.RuneCountInString(w) < relatedMinWord || strings.IndexFunc(w, unicode.IsLetter) < 0 {
				continue
			}
			m["w:"+w] += times
		}
	}
	add(n.Title, 2)
	add(n.Text, 1)
	return m
}

// weigh turns counts into TF-IDF weights in place, drops features found in
// fewer than minDF or more than maxDF notes, keeps the keep heaviest (0:
// all) and scales each note's weights to length 1. sublinear dampens
// repeated words: ten mentions are not ten times the evidence.
func weigh(docs []map[string]float64, minDF, maxDF, keep int, sublinear bool) {
	df := map[string]int{}
	for _, m := range docs {
		for k := range m {
			df[k]++
		}
	}
	n := float64(len(docs))
	for _, m := range docs {
		type kw struct {
			k string
			w float64
		}
		var ws []kw
		for k, c := range m {
			if df[k] < minDF || df[k] > maxDF {
				continue
			}
			tf := c
			if sublinear {
				tf = 1 + math.Log(c)
			}
			// +1 so a feature every note has still counts a little.
			ws = append(ws, kw{k, tf * (math.Log(n/float64(df[k])) + 1)})
		}
		sort.Slice(ws, func(a, b int) bool {
			if ws[a].w != ws[b].w {
				return ws[a].w > ws[b].w
			}
			return ws[a].k < ws[b].k
		})
		if keep > 0 && len(ws) > keep {
			ws = ws[:keep]
		}
		var norm float64
		for _, x := range ws { // sorted, so the sum is the same every time
			norm += x.w * x.w
		}
		norm = math.Sqrt(norm)
		clear(m)
		for _, x := range ws {
			m[x.k] = x.w / norm
		}
	}
}

// appendScaled adds a kind's unit vector, scaled by √weight so that the dot
// product of two notes' parts is weight × cosine.
func appendScaled(v []feature, m map[string]float64, ids map[string]int, weight float64) []feature {
	s := math.Sqrt(weight)
	for k, w := range m {
		v = append(v, feature{id: ids[k], weight: w * s})
	}
	return v
}
