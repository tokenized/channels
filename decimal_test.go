package channels

import (
	"encoding/json"
	"testing"
)

type decimalTestStruct struct {
	Decimal Decimal `json:"decimal"`
}

func Test_Decimal(t *testing.T) {
	tests := []struct {
		text  string
		value Decimal
	}{
		{
			text: "100",
			value: Decimal{
				value:     100,
				precision: 0,
			},
		},
		{
			text: "100.1",
			value: Decimal{
				value:     1001,
				precision: 1,
			},
		},
		{
			text: "0.1",
			value: Decimal{
				value:     1,
				precision: 1,
			},
		},
		{
			text: "0.01",
			value: Decimal{
				value:     1,
				precision: 2,
			},
		},
		{
			text: "100000000.00000000",
			value: Decimal{
				value:     10000000000000000,
				precision: 8,
			},
		},
		{
			text: "0.00000010",
			value: Decimal{
				value:     10,
				precision: 8,
			},
		},
		{
			text: "0.123456789",
			value: Decimal{
				value:     123456789,
				precision: 9,
			},
		},
		{
			text: "1234.56789",
			value: Decimal{
				value:     123456789,
				precision: 5,
			},
		},
		{
			text: "125.99",
			value: Decimal{
				value:     12599,
				precision: 2,
			},
		},
		{
			text: "0.99",
			value: Decimal{
				value:     99,
				precision: 2,
			},
		},
		{
			text: "10.99",
			value: Decimal{
				value:     1099,
				precision: 2,
			},
		},
		{
			text: "0.00",
			value: Decimal{
				value:     0,
				precision: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			s := tt.value.String()
			t.Logf("String : %s", s)

			if s != tt.text {
				t.Errorf("Wrong string value : got %s, want %s", s, tt.text)
			}

			var v Decimal
			if err := v.SetString(tt.text); err != nil {
				t.Errorf("Failed to set string : %s", err)
			}

			if !v.Equal(tt.value) {
				t.Errorf("Wrong value from string : got (value %d, precision %d), want (value %d, precision %d)",
					v.value, v.precision, tt.value.value, tt.value.precision)
			}

			testStruct := &decimalTestStruct{
				Decimal: tt.value,
			}
			js, err := json.Marshal(testStruct)
			if err != nil {
				t.Errorf("Failed to marshal json : %s", err)
			}
			t.Logf("JSON : %s", js)

			readStruct := &decimalTestStruct{}
			if err := json.Unmarshal(js, readStruct); err != nil {
				t.Errorf("Failed to unmarshal json : %s", err)
			}

			if !readStruct.Decimal.Equal(tt.value) {
				t.Errorf("Wrong json value : got (value %d, precision %d), want (value %d, precision %d)",
					readStruct.Decimal.value, readStruct.Decimal.precision, tt.value.value,
					tt.value.precision)
			}

			b, err := tt.value.MarshalBinary()
			if err != nil {
				t.Errorf("Failed to marshal binary : %s", err)
			}
			t.Logf("Binary : %x", b)

			var bv Decimal
			if err := bv.UnmarshalBinary(b); err != nil {
				t.Errorf("Failed to unmarshal binary : %s", err)
			}

			if !bv.Equal(tt.value) {
				t.Errorf("Wrong binary value : got (value %d, precision %d), want (value %d, precision %d)",
					bv.value, bv.precision, tt.value.value, tt.value.precision)
			}
		})
	}
}
