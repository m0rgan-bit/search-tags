package normalize

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already normal", "платье", "платье"},
		{"upper case cyrillic", "Платье", "платье"},
		{"trailing space", "Платье ", "платье"},
		{"leading and trailing", "   платье\t", "платье"},
		{"inner repeated spaces", "красное    платье", "красное платье"},
		{"tabs and newlines inside", "красное\t\n платье", "красное платье"},
		{"non-breaking space", "красное\u00a0платье", "красное платье"},
		{"mixed everything", "  КРАСНОЕ   Платье \n", "красное платье"},
		{"yo", "ЁЖ", "ёж"},
		{"latin", "  iPhone   15 ", "iphone 15"},
		{"empty", "", ""},
		{"only spaces", " \t \u00a0 ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.in); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeIdempotent(t *testing.T) {
	in := "  Вечернее   ПЛАТЬЕ "
	once := Normalize(in)
	if twice := Normalize(once); twice != once {
		t.Errorf("not idempotent: %q -> %q", once, twice)
	}
}
