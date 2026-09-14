package httpapi

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"strings"
)

type taskStepInputError struct {
	key   string
	cause error
}

func (e *taskStepInputError) Error() string { return "TASK_STEP_INPUT_INVALID:" + e.key }
func (e *taskStepInputError) Unwrap() error { return e.cause }

// Return locations and constraint names, never the validator's raw message:
// that message can echo model parameters or other author-supplied values.
func taskValidationIssue(err error) gin.H {
	issue := gin.H{"code": err.Error()}
	var step *taskStepInputError
	if !errors.As(err, &step) {
		return issue
	}
	issue["stepKey"] = step.key
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return issue
	}
	fields := []gin.H{}
	var visit func(*jsonschema.ValidationError)
	visit = func(v *jsonschema.ValidationError) {
		if len(fields) >= 32 {
			return
		}
		if len(v.Causes) > 0 {
			for _, child := range v.Causes {
				visit(child)
			}
			return
		}
		segments := []string{"with"}
		for _, part := range v.InstanceLocation {
			segments = append(segments, strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1"))
		}
		fields = append(fields, gin.H{"path": "/" + strings.Join(segments, "/"), "constraint": strings.Join(v.ErrorKind.KeywordPath(), "/")})
	}
	visit(validation)
	issue["fields"] = fields
	return issue
}
