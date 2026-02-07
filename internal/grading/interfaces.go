package grading

import "context"

type Grader interface {
	Grade(ctx context.Context, req Request) (Result, error)
}
