package handler

// Error represents an error with the intent to be sent in the HTTP
// response to the client. Therefore, it also contains a HTTPResponse,
// next to an error code and error message.
type Error struct {
	ErrorCode    string
	Message      string
	HTTPResponse HTTPResponse
}

func (e Error) Error() string {
	return e.ErrorCode + ": " + e.Message
}

func (e1 Error) Is(target error) bool {
	e2, ok := target.(Error)
	return ok && e1.ErrorCode == e2.ErrorCode
}

// NewError constructs a new Error object with the given error code and message.
// The corresponding HTTP response will have the provided status code
// and a body consisting of the error details.
// responses. See the net/http package for standardized status codes.
func NewError(errCode string, message string, statusCode int) Error {
	return Error{
		ErrorCode: errCode,
		Message:   message,
		HTTPResponse: HTTPResponse{
			StatusCode: statusCode,
			Body:       errCode + ": " + message + "\n",
			Header: HTTPHeader{
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
	}
}
