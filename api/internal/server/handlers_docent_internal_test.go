package server

import "testing"

func TestParseDocentStory_OmittedGeometryStaysOmitted(t *testing.T) {
	// Regression test: title-block and note elements don't require explicit
	// geometry. Flooring an omitted (zero) w/h to the minimum size instead of
	// leaving it at zero rendered every such element as a near-invisible
	// 10px box (e.g. a title squeezed to one word per line).
	content := `{
	  "title": "t", "date": "d",
	  "steps": [
	    {"narration": "n1", "elements": [
	      {"id": "tb", "type": "title-block", "text": "Title", "subtitle": "Sub"}
	    ]},
	    {"narration": "n2", "elements": [
	      {"id": "a", "type": "card", "x": 10, "y": 10, "w": 100, "h": 50, "text": "a"}
	    ]},
	    {"narration": "n3", "elements": [
	      {"id": "b", "type": "card", "x": 10, "y": 10, "w": 100, "h": 50, "text": "b"}
	    ]}
	  ]
	}`
	story, err := parseDocentStory(content)
	if err != nil {
		t.Fatalf("parseDocentStory: %v", err)
	}
	titleBlock := story.Steps[0].Elements[0]
	if titleBlock.W != 0 || titleBlock.H != 0 {
		t.Errorf("title-block with omitted geometry got clamped to w=%v h=%v, want 0, 0", titleBlock.W, titleBlock.H)
	}

	card := story.Steps[1].Elements[0]
	if card.W != 100 || card.H != 50 {
		t.Errorf("card with explicit geometry was altered: w=%v h=%v, want 100, 50", card.W, card.H)
	}
}

func TestParseDocentStory_MinimumStepsEnforced(t *testing.T) {
	// Regression test for the `len(story.Steps) < docentMinSteps &&
	// len(story.Steps) == 0` guard, which could only ever fire at exactly
	// zero steps — silently accepting 1 or 2 steps despite docentMinSteps
	// being 3.
	content := `{"title":"t","date":"d","steps":[
	  {"narration":"only one step","elements":[{"id":"a","type":"card","x":1,"y":1,"w":10,"h":10,"text":"a"}]}
	]}`
	if _, err := parseDocentStory(content); err == nil {
		t.Fatalf("expected an error for a story with fewer than %d steps, got none", docentMinSteps)
	}
}
