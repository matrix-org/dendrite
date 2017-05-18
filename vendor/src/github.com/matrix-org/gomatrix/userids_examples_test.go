package gomatrix

import "fmt"

func ExampleEncodeUserLocalpart() {
	localpart := EncodeUserLocalpart("Alph@Bet_50up")
	fmt.Println(localpart)
	// Output: _alph=40_bet__50up
}

func ExampleDecodeUserLocalpart() {
	localpart, err := DecodeUserLocalpart("_alph=40_bet__50up")
	if err != nil {
		panic(err)
	}
	fmt.Println(localpart)
	// Output: Alph@Bet_50up
}

func ExampleExtractUserLocalpart() {
	localpart, err := ExtractUserLocalpart("@alice:matrix.org")
	if err != nil {
		panic(err)
	}
	fmt.Println(localpart)
	// Output: alice
}
