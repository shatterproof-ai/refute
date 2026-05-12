package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	result := computeResult()
	println(msg, result)
	var u *util.User = util.NewUser("world")
	println(u.Name)
}

func computeResult() int {
	result := 6*7 + 1
	return result
}
