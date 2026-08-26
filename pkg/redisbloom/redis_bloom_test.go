package redisbloom

import "testing"

func TestParseBoolReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   interface{}
		want    bool
		wantErr bool
	}{
		{name: "RESP3 true", value: true, want: true},
		{name: "RESP3 false", value: false, want: false},
		{name: "RESP2 one", value: int64(1), want: true},
		{name: "RESP2 zero", value: int64(0), want: false},
		{name: "bulk one", value: []byte("1"), want: true},
		{name: "unsupported", value: struct{}{}, wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseBoolReply(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseBoolReply(%T) error = %v, wantErr %t", test.value, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parseBoolReply(%T) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}
