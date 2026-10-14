package bot

import "testing"

func TestPersonaDefaultsCanReadOwnProfile(t *testing.T) {
	for _, item := range defaults() {
		if item.Kind != KindPersona {
			continue
		}
		found := false
		for _, tool := range item.Tools {
			if tool == ToolGetPerson {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("分身 %s 缺少 get_person", item.BotKey)
		}
	}
}
