package domain

import (
	"testing"
	"time"
)

func TestEvaluateForecast_Table(t *testing.T) {
	cases := []struct {
		name string
		days []Day
		want map[string]string // kind → severity
	}{
		{"calm", []Day{{TempMaxC10: 300, RainMM10: 100, HumidityPct: 60, TempMinC10: 220}}, map[string]string{}},
		{"heavy rain", []Day{{RainMM10: 499}, {RainMM10: 500}}, map[string]string{KindHeavyRain: SeverityWarning}},
		{"extreme rain", []Day{{RainMM10: 1000}}, map[string]string{KindHeavyRain: SeverityCritical}},
		{"heat", []Day{{TempMaxC10: 350}}, map[string]string{KindHeat: SeverityWarning}},
		{"extreme heat", []Day{{TempMaxC10: 381}}, map[string]string{KindHeat: SeverityCritical}},
		{"blast", []Day{{HumidityPct: 85, TempMinC10: 240}}, map[string]string{KindBlastRisk: SeverityWarning}},
		{"too cold for blast", []Day{{HumidityPct: 95, TempMinC10: 190}}, map[string]string{}},
		{"too hot for blast", []Day{{HumidityPct: 95, TempMinC10: 261}}, map[string]string{}},
		{"all", []Day{{RainMM10: 600, TempMaxC10: 360, HumidityPct: 90, TempMinC10: 250}},
			map[string]string{KindHeavyRain: SeverityWarning, KindHeat: SeverityWarning, KindBlastRisk: SeverityWarning}},
	}
	for _, c := range cases {
		got := map[string]string{}
		for _, cand := range EvaluateForecast(c.days) {
			got[cand.Kind] = cand.Severity
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: %v", c.name, got)
		}
		for k, s := range c.want {
			if got[k] != s {
				t.Errorf("%s: %s=%s want %s", c.name, k, got[k], s)
			}
		}
	}
	rain := EvaluateForecast([]Day{{RainMM10: 642}})[0]
	if rain.Params["mm"] != "64" {
		t.Fatal(rain.Params)
	}
}

func TestEvaluateScanAndBuild(t *testing.T) {
	if len(EvaluateScan("completed", "healthy")) != 0 || len(EvaluateScan("low_confidence", "tungro")) != 0 || len(EvaluateScan("completed", "")) != 0 {
		t.Fatal()
	}
	c := EvaluateScan("completed", "tungro")
	if len(c) != 1 || c[0].Params["disease"] != "disease.tungro.name" {
		t.Fatal(c)
	}
	a := c[0].Build("id", "u", "e", time.Unix(0, 0))
	if a.TitleKey != "alerts.disease_followup.title" || a.BodyKey != "alerts.disease_followup.body" || a.Severity != SeverityInfo {
		t.Fatal(a)
	}
	if CellKey(24.85, 89.37) != "24.9:89.4" || len(Kinds()) != 4 || len(Errors()) != 2 {
		t.Fatal(CellKey(24.85, 89.37))
	}
}
