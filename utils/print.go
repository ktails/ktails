package utils

import (
	"encoding/json"
	"fmt"
)

func PrintJSON(d any) {
	out, err := json.Marshal(d)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(out))
}
