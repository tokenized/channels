package channels

import (
	"encoding/json"
	"fmt"
	"testing"
)

func Test_Period(t *testing.T) {
	tests := []struct {
		s string
		v Period
	}{
		{
			s: "",
			v: Period{
				Count: 0,
				Type:  PeriodTypeUnspecified,
			},
		},
		{
			s: "5 days",
			v: Period{
				Count: 5,
				Type:  PeriodTypeDay,
			},
		},
		{
			s: "week",
			v: Period{
				Count: 1,
				Type:  PeriodTypeWeek,
			},
		},
		{
			s: "month",
			v: Period{
				Count: 1,
				Type:  PeriodTypeMonth,
			},
		},
		{
			s: "2 months",
			v: Period{
				Count: 2,
				Type:  PeriodTypeMonth,
			},
		},
		{
			s: "2 years",
			v: Period{
				Count: 2,
				Type:  PeriodTypeYear,
			},
		},
		{
			s: "100 seconds",
			v: Period{
				Count: 100,
				Type:  PeriodTypeSecond,
			},
		},
		{
			s: "1000 minutes",
			v: Period{
				Count: 1000,
				Type:  PeriodTypeMinute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			t.Logf("String : %s", tt.v.String())
			if tt.v.String() != tt.s {
				t.Errorf("Wrong string value : got %s, want %s", tt.v.String(), tt.s)
			}

			js, err := json.Marshal(tt.v)
			if err != nil {
				t.Fatalf("Failed to marshal json : %s", err)
			}
			t.Logf("JSON : %s", js)

			if tt.v.Type == PeriodTypeUnspecified {
				if string(js) != "null" {
					t.Errorf("Wrong JSON : got %s, want %s", js, "null")
				}
			} else if string(js) != fmt.Sprintf("\"%s\"", tt.s) {
				t.Errorf("Wrong JSON : got %s, want %s", js, fmt.Sprintf("\"%s\"", tt.s))
			}

			v := &Period{}
			if err := json.Unmarshal(js, v); err != nil {
				t.Fatalf("Failed to unmarshal json : %s", err)
			}

			if v.Count != tt.v.Count {
				t.Errorf("Wrong json unmarshal count : got %d, want %d", v.Count, tt.v.Count)
			}
			if v.Type != tt.v.Type {
				t.Errorf("Wrong json unmarshal type : got %d, want %d", v.Type, tt.v.Type)
			}

			b, err := tt.v.MarshalBinary()
			if err != nil {
				t.Fatalf("Failed to marshal binary : %s", err)
			}
			t.Logf("Binary : 0x%x", b)

			v = &Period{}
			if err := v.UnmarshalBinary(b); err != nil {
				t.Fatalf("Failed to unmarshal binary : %s", err)
			}

			if v.Count != tt.v.Count {
				t.Errorf("Wrong binary unmarshal count : got %d, want %d", v.Count, tt.v.Count)
			}
			if v.Type != tt.v.Type {
				t.Errorf("Wrong binary unmarshal type : got %d, want %d", v.Type, tt.v.Type)
			}
		})
	}
}
