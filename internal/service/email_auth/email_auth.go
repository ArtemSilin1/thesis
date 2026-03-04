package email_auth

import (
	"fmt"
	"math/rand"
)

func createCode() int {
	return rand.Intn(900000) + 100000
}

func SendCode() {
	code := createCode()

	fmt.Println("Code:", code)

	// TODO: create email auth
}
