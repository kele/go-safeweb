// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package safehttp

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	txttemplate "text/template"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
)

type dispatcher struct{}

func (d *dispatcher) Write(rw http.ResponseWriter, resp Response) error {
	switch x := resp.(type) {
	case safehtml.HTML:
		_, err := rw.Write([]byte(x.String()))
		return err
	default:
		panic("not a safe response type")
	}
}

func (d *dispatcher) ExecuteTemplate(rw http.ResponseWriter, t Template, data interface{}) error {
	switch x := t.(type) {
	case *template.Template:
		return x.Execute(rw, data)
	default:
		panic("not a safe response type")
	}
}

func TestFormValidInt(t *testing.T) {
	stringMaxInt := strconv.FormatInt(math.MaxInt64, 10)
	tests := []struct {
		name string
		req  *http.Request
		want int64
	}{
		{
			name: "Valid int in POST non-multipart request",
			req: func() *http.Request {
				postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza="+stringMaxInt))
				postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return postReq
			}(),
			want: math.MaxInt64,
		},
		{
			name: "Valid int in POST multipart request",
			req: func() *http.Request {
				multipartReqBody := "--123\r\n" +
					"Content-Disposition: form-data; name=\"pizza\"\r\n" +
					"\r\n" +
					stringMaxInt + "\r\n" +
					"--123--\r\n"
				multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
				multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
				return multipartReq
			}(),
			want: math.MaxInt64,
		},
	}

	for _, test := range tests {
		m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
			var form *Form
			if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
				var err error
				form, err = ir.PostForm()
				if err != nil {
					t.Fatalf(`ir.PostForm: got %v, want nil`, err)
				}
			} else {
				mf, err := ir.MultipartForm(32 << 20)
				if err != nil {
					t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
				}
				form = &mf.Form
			}
			got := form.Int64("pizza", 0)
			if err := form.Err(); err != nil {
				t.Errorf(`form.Error: got %v, want nil`, err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("form.Int64: got %v, want %v, diff (-want +got): \n%s", got, test.want, diff)
			}
			return Result{}
		}, &dispatcher{})
		recorder := httptest.NewRecorder()
		m.HandleRequest(recorder, test.req)
		if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
			t.Errorf("response status: got %v, want %v", respStatus, want)
		}
	}
}

func TestFormInvalidInt(t *testing.T) {
	tests := []struct {
		name string
		reqs []*http.Request
		err  error
		want int64
	}{
		{
			name: "Overflow integer in request",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=9223372036854775810"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"9223372036854775810\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err:  errors.New(`strconv.ParseInt: parsing "9223372036854775810": value out of range`),
			want: 0,
		},
		{
			name: "Not an integer in request",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=diavola"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq

				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"diavola\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err:  errors.New(`strconv.ParseInt: parsing "diavola": invalid syntax`),
			want: 0,
		},
	}

	for _, test := range tests {
		for _, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				got := form.Int64("pizza", 0)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("form.Int64: got %v, want %v, diff (-want +got): \n%s", got, test.want, diff)
				}
				if form.Err() == nil {
					t.Errorf("form.Err: got nil, want %v", test.err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormValidUint(t *testing.T) {
	stringMaxUint := strconv.FormatUint(math.MaxUint64, 10)
	tests := []struct {
		name string
		req  *http.Request
		want uint64
	}{
		{
			name: "Valid unsigned integer in POST non-multipart request",
			req: func() *http.Request {
				postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza="+stringMaxUint))
				postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return postReq
			}(),
			want: math.MaxUint64,
		},
		{
			name: "Valid unsigned integer in POST multipart request",
			req: func() *http.Request {
				multipartReqBody := "--123\r\n" +
					"Content-Disposition: form-data; name=\"pizza\"\r\n" +
					"\r\n" +
					stringMaxUint + "\r\n" +
					"--123--\r\n"
				multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
				multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
				return multipartReq
			}(),
			want: math.MaxUint64,
		},
	}

	for _, test := range tests {
		m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
			var form *Form
			if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
				var err error
				form, err = ir.PostForm()
				if err != nil {
					t.Fatalf(`ir.PostForm: got %v, want nil`, err)
				}
			} else {
				mf, err := ir.MultipartForm(32 << 20)
				if err != nil {
					t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
				}
				form = &mf.Form
			}
			got := form.Uint64("pizza", 0)
			if err := form.Err(); err != nil {
				t.Errorf(`form.Error: got %v, want nil`, err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("form.Uint64: got %v, want %v, diff (-want +got): \n%s", got, test.want, diff)
			}
			return Result{}
		}, &dispatcher{})
		recorder := httptest.NewRecorder()
		m.HandleRequest(recorder, test.req)
		if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
			t.Errorf("response status: got %v, want %v", respStatus, want)
		}
	}
}

func TestFormInvalidUint(t *testing.T) {
	tests := []struct {
		name string
		reqs []*http.Request
		err  error
		want uint64
	}{
		{
			name: "Not an unsigned integer",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=-1"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"-1\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err:  errors.New(`strconv.ParseUint: parsing "-1": invalid syntax`),
			want: 0,
		},
		{
			name: "Overflow unsigned integer",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=18446744073709551630"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"18446744073709551630\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err:  errors.New(`strconv.ParseUint: parsing "18446744073709551630": value out of range`),
			want: 0,
		},
	}
	for _, test := range tests {
		for _, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				got := form.Uint64("pizza", 0)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("form.Uint64: got %v, want %v, diff (-want +got): \n%s", got, test.want, diff)
				}
				if form.Err() == nil {
					t.Errorf("form.Err: got nil, want %v", test.err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormValidString(t *testing.T) {
	tests := []struct {
		name string
		reqs []*http.Request
		want []string
	}{
		{
			name: "Valid string in POST non-multipart request",
			reqs: func() []*http.Request {
				reqs := []*http.Request{
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=diavola")),
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=ăȚâȘî")),
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=\x64\x69\x61\x76\x6f\x6c\x61")),
				}
				for _, req := range reqs {
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				return reqs
			}(),
			want: []string{"diavola", "ăȚâȘî", "diavola"},
		},
		{
			name: "Valid string in POST multipart request",
			reqs: func() []*http.Request {
				slice := []string{"diavola", "ăȚâȘî", "diavola"}
				var reqs []*http.Request
				for _, val := range slice {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						val + "\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					reqs = append(reqs, multipartReq)

				}
				return reqs
			}(),
			want: []string{"diavola", "ăȚâȘî", "diavola"},
		},
	}

	for _, test := range tests {
		for idx, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				want := test.want[idx]
				got := form.String("pizza", "")
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("form.String: got %v, want %v, diff (-want +got): \n%s", got, want, diff)
				}
				if err := form.Err(); err != nil {
					t.Errorf("form.Err: got %v, want nil", err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormValidFloat64(t *testing.T) {
	stringMaxFloat := strconv.FormatFloat(math.MaxFloat64, 'f', 6, 64)
	stringNegativeFloat := strconv.FormatFloat(-math.SmallestNonzeroFloat64, 'f', 324, 64)
	tests := []struct {
		name string
		reqs []*http.Request
		want []float64
	}{
		{
			name: "Valid floats in POST non-multipart request",
			reqs: func() []*http.Request {
				reqs := []*http.Request{
					httptest.NewRequest("POST", "/", strings.NewReader("pizza="+stringMaxFloat)),
					httptest.NewRequest("POST", "/", strings.NewReader("pizza="+stringNegativeFloat)),
				}
				for _, req := range reqs {
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				return reqs
			}(),
			want: []float64{math.MaxFloat64, -math.SmallestNonzeroFloat64},
		},
		{
			name: "Valid floats in POST multipart request",
			reqs: func() []*http.Request {
				slice := []string{stringMaxFloat, stringNegativeFloat}
				var reqs []*http.Request
				for _, val := range slice {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						val + "\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					reqs = append(reqs, multipartReq)

				}
				return reqs
			}(),
			want: []float64{math.MaxFloat64, -math.SmallestNonzeroFloat64},
		},
	}

	for _, test := range tests {
		for idx, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				want := test.want[idx]
				got := form.Float64("pizza", 0.0)
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("form.Float64: got %v, want %v, diff (-want +got): \n%s", got, want, diff)
				}
				if err := form.Err(); err != nil {
					t.Errorf("form.Err: got %v, want nil", err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormInvalidFloat64(t *testing.T) {
	tests := []struct {
		name string
		reqs []*http.Request
		err  error
		want float64
	}{
		{
			name: "Not a float64 in request",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=diavola"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"diavola\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err:  errors.New(`strconv.ParseFloat: parsing "diavola": invalid syntax`),
			want: 0.0},
		{
			name: "Overflow float64 in request",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=1.797693134862315708145274237317043567981e309"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"1.797693134862315708145274237317043567981e309\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err:  errors.New(`strconv.ParseFloat: parsing "1.797693134862315708145274237317043567981e309": value out of range`),
			want: 0.0,
		},
	}
	for _, test := range tests {
		for _, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				got := form.Float64("pizza", 0.0)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("form.Float64: got %v, want %v, diff (-want +got): \n%s", got, test.want, diff)
				}
				if form.Err() == nil {
					t.Errorf("form.Err: got nil, want %v", test.err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormValidBool(t *testing.T) {
	tests := []struct {
		name string
		reqs []*http.Request
		want []bool
	}{
		{
			name: "Valid booleans in POST non-multipart request",
			reqs: func() []*http.Request {
				reqs := []*http.Request{
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=true")),
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=false")),
				}
				for _, req := range reqs {
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				return reqs
			}(),
			want: []bool{true, false},
		},
		{
			name: "Valid booleans in POST multipart request",
			reqs: func() []*http.Request {
				slice := []bool{true, false}
				var reqs []*http.Request
				for _, val := range slice {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						fmt.Sprintf("%v\r\n", val) +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					reqs = append(reqs, multipartReq)

				}
				return reqs
			}(),
			want: []bool{true, false},
		},
	}

	for _, test := range tests {
		for idx, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				want := test.want[idx]
				got := form.Bool("pizza", false)
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("form.Bool: got %v, want %v, diff (-want +got): \n%s", got, want, diff)
				}
				if err := form.Err(); err != nil {
					t.Errorf("form.Err: got %v, want nil", err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormInvalidBool(t *testing.T) {
	tests := []struct {
		name string
		reqs []*http.Request
		want bool
		err  error
	}{
		{
			name: "Invalid booleans in POST non-multipart request",
			reqs: func() []*http.Request {
				reqs := []*http.Request{
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=TruE")),
					httptest.NewRequest("POST", "/", strings.NewReader("pizza=potato")),
				}
				for _, req := range reqs {
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				return reqs
			}(),
			want: false,
			err:  errors.New(`values of form parameter "pizza" not a boolean`),
		},
		{
			name: "Invalid booleans in POST multipart request",
			reqs: func() []*http.Request {
				slice := []string{"TruE", "potato"}
				var reqs []*http.Request
				for _, val := range slice {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						val + "\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					reqs = append(reqs, multipartReq)

				}
				return reqs
			}(),
			want: false,
			err:  errors.New(`values of form parameter "pizza" not a boolean`),
		},
	}

	for _, test := range tests {
		for _, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				got := form.Bool("pizza", false)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("form.Bool: got %v, want %v, diff (-want +got): \n%s", got, test.want, diff)
				}
				if form.Err() == nil {
					t.Errorf("form.Err: got nil, want %v", test.err)
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormValidSliceMultipart(t *testing.T) {
	multipartBody := txttemplate.Must(txttemplate.New("multipart").Parse(
		"{{ range . -}}" +
			"--123\r\n" +
			"Content-Disposition: form-data name=\"pizza\"\r\n" +
			"\r\n" +
			"{{ . }}\r\n" +
			"{{ end }}" +
			"--123--\r\n"))

	tests := []struct {
		str         []string
		req         *http.Request
		placeholder interface{}
		want        interface{}
	}{
		{
			str:         []string{"-8", "9", "-100"},
			placeholder: []int64{},
			want:        []int64{-8, 9, -100},
		},
		{
			str:         []string{"8", "9", "10"},
			placeholder: []uint64{},
			want:        []uint64{8, 9, 10},
		},
		{
			str:         []string{"margeritta", "diavola", "calzone"},
			placeholder: []string{},
			want:        []string{"margeritta", "diavola", "calzone"},
		},
		{
			str:         []string{"1.3", "8.9", "-4.1"},
			placeholder: []float64{},
			want:        []float64{1.3, 8.9, -4.1},
		},
		{
			str:         []string{"true", "false", "true"},
			placeholder: []bool{},
			want:        []bool{true, false, true},
		},
	}
	for _, tc := range tests {
		var body *strings.Builder
		multipartBody.Execute(body, tc.str)
		multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(body.String()))
		multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
	}

	for _, tc := range tests {
		m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
			form, err := ir.MultipartForm(32 << 20)
			if err != nil {
				t.Fatalf(`ir.MultipartForm: got err %v, want nil`, err)
			}
			got := &tc.placeholder
			form.Slice("pizza", got)
			if err := form.Err(); err != nil {
				t.Errorf(`form.Err: got err %v, want nil`, err)
			}
			if diff := cmp.Diff(tc.want, *got); diff != "" {
				t.Errorf("form.Slice: got %v, want %v, diff (-want +got): \n%s", *got, tc.want, diff)
			}
			return Result{}
		}, &dispatcher{})
		recorder := httptest.NewRecorder()
		m.HandleRequest(recorder, tc.req)
		if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
			t.Errorf("response status: got %v, want %v", respStatus, want)
		}
	}
}

func TestFormInvalidSlice(t *testing.T) {
	// Parsing behaviour of native types that are supported is not included as
	// it was tested in previous tests.
	tests := []struct {
		name string
		reqs []*http.Request
		err  error
		got  interface{}
		want interface{}
	}{
		{
			name: "Request with multiple types in slice",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=true&pizza=1.3"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"true\r\n" +
						"--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"1.3\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err: errors.New(`values of form parameter "pizza" not a boolean`),
			got: func() interface{} {
				var got []bool
				return got
			}(),
			want: func() interface{} {
				var want []bool
				return want
			}(),
		},
		{
			name: "Unsupported slice type",
			reqs: []*http.Request{
				func() *http.Request {
					postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizza=true&pizza=1.3"))
					postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return postReq
				}(),
				func() *http.Request {
					multipartReqBody := "--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"true\r\n" +
						"--123\r\n" +
						"Content-Disposition: form-data; name=\"pizza\"\r\n" +
						"\r\n" +
						"1.3\r\n" +
						"--123--\r\n"
					multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
					multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
					return multipartReq
				}(),
			},
			err: errors.New(`type not supported in Slice call: *[]int8`),
			got: func() interface{} {
				var got []int8
				return got
			}(),
			want: func() interface{} {
				var want []int8
				return want
			}(),
		},
	}

	for _, test := range tests {
		for _, req := range test.reqs {
			m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
				var form *Form
				if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
					var err error
					form, err = ir.PostForm()
					if err != nil {
						t.Fatalf(`ir.PostForm: got %v, want nil`, err)
					}
				} else {
					mf, err := ir.MultipartForm(32 << 20)
					if err != nil {
						t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
					}
					form = &mf.Form
				}
				switch got := test.got.(type) {
				case []int8:
					form.Slice("pizza", &got)
					if diff := cmp.Diff(test.want, got); diff != "" {
						t.Errorf("form.Slice: got %v, want %v", got, test.want)
					}
					if form.Err() == nil {
						t.Errorf("form.Err: got nil, want %v", test.err)
					}
				case []bool:
					form.Slice("pizza", &got)
					if diff := cmp.Diff(test.want, got); diff != "" {
						t.Errorf("form.Slice: got %v, want %v", got, test.want)
					}
					if form.Err() == nil {
						t.Errorf("form.Err: got nil, want %v", test.err)
					}
				}
				return Result{}
			}, &dispatcher{})
			recorder := httptest.NewRecorder()
			m.HandleRequest(recorder, req)
			if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
				t.Errorf("response status: got %v, want %v", respStatus, want)
			}
		}
	}
}

func TestFormErrorHandling(t *testing.T) {
	test := struct {
		name string
		reqs []*http.Request
		errs []error
	}{
		name: "Erros occuring in requests",
		reqs: []*http.Request{
			func() *http.Request {
				postReq := httptest.NewRequest("POST", "/", strings.NewReader("pizzaInt=diavola&pizzaBool=true&pizzaUint=-13"))
				postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return postReq
			}(),
			func() *http.Request {
				multipartReqBody := "--123\r\n" +
					"Content-Disposition: form-data; name=\"pizzaInt\"\r\n" +
					"\r\n" +
					"diavola\r\n" +
					"--123\r\n" +
					"Content-Disposition: form-data; name=\"pizzaBool\"\r\n" +
					"\r\n" +
					"true\r\n" +
					"--123\r\n" +
					"Content-Disposition: form-data; name=\"pizzaUint\"\r\n" +
					"\r\n" +
					"-13\r\n" +
					"--123--\r\n"
				multipartReq := httptest.NewRequest("POST", "/", strings.NewReader(multipartReqBody))
				multipartReq.Header.Set("Content-Type", `multipart/form-data; boundary="123"`)
				return multipartReq
			}(),
		},
		errs: []error{errors.New(`strconv.ParseInt: parsing "diavola": invalid syntax`), errors.New(`strconv.ParseUint: parsing "-13": invalid syntax`)},
	}

	for _, req := range test.reqs {
		m := NewMachinery(func(rw ResponseWriter, ir *IncomingRequest) Result {
			var form *Form
			if !strings.HasPrefix(ir.req.Header.Get("Content-Type"), "multipart/form-data") {
				var err error
				form, err = ir.PostForm()
				if err != nil {
					t.Fatalf(`ir.PostForm: got %v, want nil`, err)
				}
			} else {
				mf, err := ir.MultipartForm(32 << 20)
				if err != nil {
					t.Fatalf(`ir.MultipartForm: got %v, want nil`, err)
				}
				form = &mf.Form
			}
			var wantInt int64 = 0
			gotInt := form.Int64("pizzaInt", 0)
			if diff := cmp.Diff(wantInt, gotInt); diff != "" {
				t.Errorf("form.Int64: got %v, want %v, diff (-want +got): \n%s", gotInt, wantInt, diff)
			}
			if form.Err() == nil {
				t.Errorf("form.Err: got nil, want %v", test.errs[0])
			}
			wantBool := true
			gotBool := form.Bool("pizzaBool", false)
			if diff := cmp.Diff(wantBool, gotBool); diff != "" {
				t.Errorf("form.Bool: got %v, want %v, diff (-want +got): \n%s", gotBool, wantBool, diff)
			}
			// We expect the same error here becase calling form.Bool succeeds
			if form.Err() == nil {
				t.Errorf("form.Err: got nil, want %v", test.errs[0])
			}
			var wantUint uint64 = 0
			gotUint := form.Uint64("pizzaUint", 0)
			if diff := cmp.Diff(wantUint, gotUint); diff != "" {
				t.Errorf("form.Uint64: got %v, want %v, diff (-want +got): \n%s", gotUint, wantUint, diff)
			}
			if form.Err() == nil {
				t.Errorf("form.Err: got nil, want %v", test.errs[1])
			}
			return Result{}
		}, &dispatcher{})
		recorder := httptest.NewRecorder()
		m.HandleRequest(recorder, req)
		if respStatus, want := recorder.Result().StatusCode, 200; respStatus != want {
			t.Errorf("response status: got %v, want %v", respStatus, want)
		}
	}
}
