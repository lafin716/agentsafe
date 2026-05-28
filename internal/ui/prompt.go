package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func Confirm(question string, yes bool) bool {
	if yes {
		return true
	}
	fmt.Printf("%s [y/N]: ", question)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}
