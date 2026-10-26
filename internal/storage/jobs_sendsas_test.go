package storage

import "testing"

// ADR-191: a run speaks for a person when one started it, or when it is a
// scheduled automation run as administrator (for the admin who allowed it);
// never a bot's message or a webhook.
func TestJobSendsAs(t *testing.T) {
	for _, c := range []struct {
		name string
		j    Job
		as   string
	}{
		{"person", Job{CreatedBy: "human:a@x"}, "human:a@x"},
		{"schedule as administrator", Job{Origin: "automation", Trigger: "schedule", FullAccess: true, FullAccessBy: "boss@x"}, "human:boss@x"},
		{"schedule, not administrator", Job{Origin: "automation", Trigger: "schedule"}, ""},
		{"webhook", Job{Origin: "automation", Trigger: "webhook", FullAccess: true, FullAccessBy: "boss@x"}, ""},
		{"bot", Job{Origin: "automation", Trigger: "telegram", FullAccess: true, FullAccessBy: "boss@x", CreatedBy: "telegram:u"}, ""},
	} {
		as, ok := c.j.SendsAs()
		if as != c.as || ok != (c.as != "") {
			t.Errorf("%s: %q %v", c.name, as, ok)
		}
	}
}
