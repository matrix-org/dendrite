// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	 http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
)

const origin = "matrix.org"
const mediaID = "rZOBfBHnuOoyqBKUIHAaSbcM"

const requestCount = 100

func main() {
	httpURL := "http://localhost:7777/api/_matrix/media/v1/download/" + origin + "/" + mediaID
	jsonResponses := make(chan string)

	var wg sync.WaitGroup

	wg.Add(requestCount)

	for i := 0; i < requestCount; i++ {
		go func() {
			defer wg.Done()
			res, err := http.Get(httpURL)
			if err != nil {
				log.Fatal(err)
			} else {
				defer res.Body.Close()
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					log.Fatal(err)
				} else {
					if res.StatusCode != 200 {
						jsonResponses <- string(body)
					}
				}
			}
		}()
	}

	errorCount := 0
	go func() {
		for response := range jsonResponses {
			errorCount++
			fmt.Println(response)
		}
	}()

	wg.Wait()
	fmt.Printf("%v/%v requests were successful\n", requestCount-errorCount, requestCount)
}
