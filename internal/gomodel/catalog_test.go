package gomodel

import (
	"fmt"
	"sync"
	"testing"
)

func catalogFixture(id string) Info {
	return Info{ID: "cline-pass/" + id, Name: "Model " + id, Endpoint: EndpointChat}
}

func TestCatalogReplacesCompleteListAndCopiesData(t *testing.T) {
	c := NewCatalog()
	if len(c.All()) != 12 {
		t.Fatal("expected current official fallback catalog")
	}
	models := []Info{catalogFixture("new-model")}
	if err := c.Replace(models); err != nil {
		t.Fatal(err)
	}
	models[0].ID = "cline-pass/changed-input"
	listed := c.All()
	listed[0].Name = "changed-output"
	for _, id := range []string{"new-model", "cline-pass/new-model", "opencode-go/new-model", " CLINE/new-model "} {
		if m, ok := c.Lookup(id); !ok || m.Name != "Model new-model" {
			t.Fatalf("lookup %q returned %+v, %v", id, m, ok)
		}
	}
	if _, ok := c.Lookup("glm-5.3"); ok {
		t.Fatal("removed model remains in the lookup")
	}
	if len(c.All()) != 1 {
		t.Fatal("list retains removed models")
	}
}

func TestCatalogRejectsInvalidListWithoutChangingActiveModels(t *testing.T) {
	c := NewCatalog()
	before := c.All()
	for name, models := range map[string][]Info{
		"empty":         nil,
		"duplicate":     {catalogFixture("a"), catalogFixture("a")},
		"missing name":  {{ID: "cline-pass/a"}},
		"bad namespace": {{ID: "another/a", Name: "A"}},
		"bad id":        {{ID: "cline-pass/a/b", Name: "A"}},
		"bad endpoint":  {{ID: "cline-pass/a", Name: "A", Endpoint: "image"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := c.Replace(models); err == nil {
				t.Fatal("accepted invalid model catalog")
			}
			if after := c.All(); len(after) != len(before) || after[0] != before[0] {
				t.Fatal("invalid replacement changed active catalog")
			}
		})
	}
}

func TestCatalogConcurrentReadersSeeCompleteSnapshots(t *testing.T) {
	c := NewCatalog()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 300 {
				models := c.All()
				if len(models) != 12 && len(models) != 2 {
					t.Errorf("incomplete snapshot: %d models", len(models))
					return
				}
				c.Lookup("new-model")
			}
		})
	}
	wg.Go(func() {
		for i := range 300 {
			if err := c.Replace([]Info{catalogFixture("new-model"), catalogFixture(fmt.Sprintf("model-%d", i))}); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
}
