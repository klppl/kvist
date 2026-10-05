package model

import "testing"

func TestUnlisted(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Listed.md":          "---\ntags: [shared]\n---\nSee [[Hidden]].",
		"Kvist/Hidden.md":          "---\nunlisted: true\ntags: [shared, secret]\n---\nSee [[Listed]].",
		"Kvist/Drafts/Draft.md":    "---\nunlisted: \"yes\"\n---\nA draft.",
		"Kvist/Drafts/Deep/Old.md": "---\nunlisted: true\n---\nOld.",
	})
	hidden := site.Note("Kvist/Hidden.md")
	listed := site.Note("Kvist/Listed.md")
	draft := site.Note("Kvist/Drafts/Draft.md")
	if hidden == nil || !hidden.Unlisted || listed.Unlisted || !draft.Unlisted {
		t.Fatalf("unlisted flags: hidden=%v listed=%v draft=%v", hidden, listed, draft)
	}

	kvist := site.Root.Children[0]
	if len(kvist.Notes) != 1 || kvist.Notes[0] != listed {
		t.Errorf("folder notes: %v", kvist.NoteIDs)
	}
	if len(kvist.Children) != 0 {
		t.Errorf("a folder of unlisted notes is still in the tree: %v", kvist.Children[0].Path)
	}
	if draft.Folder == nil || !draft.Folder.Unlisted || draft.Folder.Name != "Drafts" || draft.Folder.Parent != kvist {
		t.Errorf("an unlisted note's folder: %+v", draft.Folder)
	}

	if len(site.AllTags) != 1 || site.AllTags[0].Name != "shared" || len(site.AllTags[0].Notes) != 1 {
		t.Errorf("tags: %+v", site.AllTags)
	}
	if len(hidden.Tags) != 1 || hidden.Tags[0].Name != "shared" {
		t.Errorf("an unlisted note keeps the tags that have pages: %v", hidden.Tags)
	}

	if len(listed.Backlinks) != 0 {
		t.Errorf("backlink from an unlisted note: %v", listed.Backlinks[0].SourceID)
	}
	if len(hidden.Backlinks) != 1 || len(listed.Links) != 1 {
		t.Errorf("links to an unlisted note still work: %v %v", hidden.Backlinks, listed.Links)
	}
	for _, n := range site.Graph.Nodes {
		if n.ID != listed.ID {
			t.Errorf("graph node %q", n.ID)
		}
	}
	if len(site.Graph.Edges) != 0 {
		t.Errorf("graph edges: %v", site.Graph.Edges)
	}
}
