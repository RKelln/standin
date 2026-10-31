package jsonsafe

import "testing"

func TestStrictJSON(t *testing.T) {
	for _, s := range []string{`{}`, `[]`, `{"name":"Zelda Quorn"}`, `{"face":"\ud83d\ude00"}`, `{"literal":"\\ud800"}`, `{"quote":"\""}`, `null`} {
		if err := Validate([]byte(s)); err != nil {
			t.Errorf("valid input refused: %q", s)
		}
	}
	for _, s := range []string{``, `{"x":1,"x":2}`, `{"nested":{"x":1,"x":2}}`, `{} {}`, `{} trailing`, `{"x":"\ud800"}`, `{"x":"\udc00"}`, `{"x":"\ud800\u0041"}`, `{"x":`, "{\"x\":\"\xff\"}"} {
		if err := Validate([]byte(s)); err == nil {
			t.Errorf("damaged input accepted: %q", s)
		}
	}
}
