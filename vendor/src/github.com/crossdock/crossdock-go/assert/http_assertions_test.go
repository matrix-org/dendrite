// Copyright (c) 2012 - 2013 Mat Ryer and Tyler Bunnell
//
// Please consider promoting this project if you find it useful.
//
// Permission is hereby granted, free of charge, to any person
// obtaining a copy of this software and associated documentation
// files (the "Software"), to deal in the Software without restriction,
// including without limitation the rights to use, copy, modify, merge,
// publish, distribute, sublicense, and/or sell copies of the Software,
// and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
// OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
// IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT
// OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE
// OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
//
package assert

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func httpOK(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func httpRedirect(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func httpError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
}

func TestHTTPStatuses(t *testing.T) {
	assert := New(t)
	mockT := new(testing.T)

	assert.Equal(HTTPSuccess(mockT, httpOK, "GET", "/", nil), true)
	assert.Equal(HTTPSuccess(mockT, httpRedirect, "GET", "/", nil), false)
	assert.Equal(HTTPSuccess(mockT, httpError, "GET", "/", nil), false)

	assert.Equal(HTTPRedirect(mockT, httpOK, "GET", "/", nil), false)
	assert.Equal(HTTPRedirect(mockT, httpRedirect, "GET", "/", nil), true)
	assert.Equal(HTTPRedirect(mockT, httpError, "GET", "/", nil), false)

	assert.Equal(HTTPError(mockT, httpOK, "GET", "/", nil), false)
	assert.Equal(HTTPError(mockT, httpRedirect, "GET", "/", nil), false)
	assert.Equal(HTTPError(mockT, httpError, "GET", "/", nil), true)
}

func TestHTTPStatusesWrapper(t *testing.T) {
	assert := New(t)
	mockAssert := New(new(testing.T))

	assert.Equal(mockAssert.HTTPSuccess(httpOK, "GET", "/", nil), true)
	assert.Equal(mockAssert.HTTPSuccess(httpRedirect, "GET", "/", nil), false)
	assert.Equal(mockAssert.HTTPSuccess(httpError, "GET", "/", nil), false)

	assert.Equal(mockAssert.HTTPRedirect(httpOK, "GET", "/", nil), false)
	assert.Equal(mockAssert.HTTPRedirect(httpRedirect, "GET", "/", nil), true)
	assert.Equal(mockAssert.HTTPRedirect(httpError, "GET", "/", nil), false)

	assert.Equal(mockAssert.HTTPError(httpOK, "GET", "/", nil), false)
	assert.Equal(mockAssert.HTTPError(httpRedirect, "GET", "/", nil), false)
	assert.Equal(mockAssert.HTTPError(httpError, "GET", "/", nil), true)
}

func httpHelloName(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	w.Write([]byte(fmt.Sprintf("Hello, %s!", name)))
}

func TestHttpBody(t *testing.T) {
	assert := New(t)
	mockT := new(testing.T)

	assert.True(HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	assert.True(HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	assert.False(HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))

	assert.False(HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	assert.False(HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	assert.True(HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))
}

func TestHttpBodyWrappers(t *testing.T) {
	assert := New(t)
	mockAssert := New(new(testing.T))

	assert.True(mockAssert.HTTPBodyContains(httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	assert.True(mockAssert.HTTPBodyContains(httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	assert.False(mockAssert.HTTPBodyContains(httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))

	assert.False(mockAssert.HTTPBodyNotContains(httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	assert.False(mockAssert.HTTPBodyNotContains(httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	assert.True(mockAssert.HTTPBodyNotContains(httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))

}
