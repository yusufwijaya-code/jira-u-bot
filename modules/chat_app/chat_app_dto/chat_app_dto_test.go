package chat_app_dto

import (
	"encoding/json"
	"testing"
)

func TestEvent_DecodesSlashCommandAsStringOrNumber(t *testing.T) {
	for _, raw := range []string{
		`{"type":"MESSAGE","space":{"name":"spaces/A","type":"DM"},"user":{"name":"users/1","email":"a@esb.co.id"},"message":{"text":"/n-help","slashCommand":{"commandId":"5"}}}`,
		`{"type":"MESSAGE","space":{"name":"spaces/A","spaceType":"DIRECT_MESSAGE"},"user":{"name":"users/1","email":"a@esb.co.id"},"message":{"text":"/n-help","slashCommand":{"commandId":5}}}`,
	} {
		var ev Event
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if !ev.Space.IsDM() {
			t.Errorf("IsDM = false for %s", raw)
		}
		if ev.Message == nil || ev.Message.SlashCommand == nil || ev.Message.SlashCommand.CommandID != "5" {
			t.Errorf("CommandID not decoded from %s: %+v", raw, ev.Message)
		}
	}
}

func TestAction_Param(t *testing.T) {
	a := &Action{Parameters: []ActionParameter{{Key: "issueKey", Value: "CORE-1"}}}
	if a.Param("issueKey") != "CORE-1" || a.Param("target") != "" {
		t.Errorf("Param mismatch: %+v", a)
	}
	var nilAction *Action
	if nilAction.Param("x") != "" {
		t.Error("nil Action.Param must return empty")
	}
}

func TestResponse_ZeroValueIsEmptyObject(t *testing.T) {
	b, err := json.Marshal(Response{})
	if err != nil || string(b) != "{}" {
		t.Errorf("Marshal(Response{}) = %s, %v; want {}", b, err)
	}
}
