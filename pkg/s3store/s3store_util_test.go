package s3store

import (
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/mock/gomock"
)

type UploadPartInputMatcher struct {
	expect *s3.UploadPartInput
}

func NewUploadPartInputMatcher(expect *s3.UploadPartInput) gomock.Matcher {
	return UploadPartInputMatcher{
		expect: expect,
	}
}

func (m UploadPartInputMatcher) Matches(x interface{}) bool {
	input, ok := x.(*s3.UploadPartInput)
	if !ok {
		return false
	}

	inputBody := input.Body
	expectBody := m.expect.Body

	i, err := ioutil.ReadAll(inputBody)
	if err != nil {
		panic(err)
	}
	inputBody.Seek(0, 0)

	e, err := ioutil.ReadAll(expectBody)
	if err != nil {
		panic(err)
	}
	m.expect.Body.Seek(0, 0)

	if !reflect.DeepEqual(e, i) {
		return false
	}

	input.Body = nil
	m.expect.Body = nil

	return reflect.DeepEqual(m.expect, input)
}

func (m UploadPartInputMatcher) String() string {
	body, _ := ioutil.ReadAll(m.expect.Body)
	m.expect.Body.Seek(0, 0)
	return fmt.Sprintf("UploadPartInput(%d: %s)", *m.expect.PartNumber, body)
}

type PutObjectInputMatcher struct {
	expect *s3.PutObjectInput
}

func NewPutObjectInputMatcher(expect *s3.PutObjectInput) gomock.Matcher {
	return PutObjectInputMatcher{
		expect: expect,
	}
}

func (m PutObjectInputMatcher) Matches(x interface{}) bool {
	input, ok := x.(*s3.PutObjectInput)
	if !ok {
		return false
	}

	inputBody := input.Body
	expectBody := m.expect.Body

	i, err := ioutil.ReadAll(inputBody)
	if err != nil {
		panic(err)
	}
	inputBody.Seek(0, 0)

	e, err := ioutil.ReadAll(expectBody)
	if err != nil {
		panic(err)
	}
	m.expect.Body.Seek(0, 0)

	if !reflect.DeepEqual(e, i) {
		return false
	}

	input.Body = nil
	m.expect.Body = nil

	return reflect.DeepEqual(m.expect, input)
}

func (m PutObjectInputMatcher) String() string {
	body, _ := ioutil.ReadAll(m.expect.Body)
	m.expect.Body.Seek(0, 0)
	return fmt.Sprintf(`PutObjectInput(Body: "%s")`, body)
}
