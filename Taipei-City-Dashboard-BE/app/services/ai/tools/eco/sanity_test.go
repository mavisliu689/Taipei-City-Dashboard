// Package eco hosts AI tools for the carbon-reduction route assistant.
// Sanity check confirms the test pipeline and testify wiring.
package eco

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanity_Should_Pass_When_TestifyWired(t *testing.T) {
	assert.Equal(t, 2, 1+1)
}
