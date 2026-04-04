package team

import (
	"fmt"
	"log"
	"strings"
)

func logTeamEvent(event string, kv ...any) {
	event = strings.TrimSpace(event)
	if event == "" {
		event = "event"
	}
	fields := formatTeamLogFields(kv...)
	if fields == "" {
		log.Printf("[team] %s", event)
		return
	}
	log.Printf("[team] %s %s", event, fields)
}

func formatTeamLogFields(kv ...any) string {
	if len(kv) == 0 {
		return ""
	}
	parts := make([]string, 0, (len(kv)+1)/2)
	for i := 0; i < len(kv); i += 2 {
		key := strings.TrimSpace(fmt.Sprint(kv[i]))
		if key == "" {
			key = "field"
		}
		value := ""
		if i+1 < len(kv) {
			value = strings.TrimSpace(fmt.Sprint(kv[i+1]))
		}
		if value == "" {
			value = "\"\""
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " ")
}
