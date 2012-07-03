package falcore

import (
	"net/http"
	"testing"
	"fmt"
	"io/ioutil"
)

func TestHandlerFuncFilter(t *testing.T) {
	reply := "Hello, World"
	handler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, reply)
	}
	
	hff := NewHandlerFuncFilter(handler)

	tmp, _ := http.NewRequest("GET", "/hello", nil)
	_, res := TestWithRequest(tmp, hff)
	
	if res == nil {
		t.Errorf("Response is nil")
	}
	
	if replyGot, err := ioutil.ReadAll(res.Body); err != nil {
		t.Errorf("Error reading body: %v", err)
	} else if string(replyGot) != reply {
		t.Errorf("Expected body does not match")
	}
	
}