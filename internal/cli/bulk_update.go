// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored for Canvas Builder: reading the list `assignments bulk-update`
// sends.
//
// Canvas's bulk_update is one of the few endpoints whose body is a JSON array
// rather than an object, and the array is load-bearing twice over. Canvas reads
// it to decide which assignments to move, and Canvas Builder reads the same
// array to decide which assignments to fetch a before-copy of, so the undo can
// put the dates back. A body that arrives as anything else costs both.
//
// This lives beside the generated command rather than inside it so the
// generated file keeps the shape the generator will write again.

package cli

import (
	"encoding/json"
	"errors"
)

// msgBulkUpdateNeedsList is what an agent is told when the body isn't a list.
// It carries a working example because the mistake is nearly always the shape
// rather than the idea: an agent that has been writing single-assignment
// updates sends one object, and one object is a different endpoint.
//
// It is a variable rather than a literal inside errors.New so the sentence can
// end in a full stop. Error strings aren't usually written as sentences, but
// this one is read by an agent in a terminal, not appended to a caller's
// prefix, and the example at the end of it is the useful part.
var msgBulkUpdateNeedsList = `bulk-update needs a JSON list such as [{"id":12,"all_dates":[{"base":true,"due_at":"2027-02-01T07:59:00Z"}]}].`

// bulkUpdateList checks that raw is a non-empty JSON list of objects and
// returns it unchanged, to go on the wire as it arrived.
//
// Passing the bytes through rather than a Go value re-marshalled is deliberate.
// A round trip through map[string]any turns every number into a float64, and
// an assignment id is the one value in this body that has to survive exactly:
// it is what the before-copy is fetched by. Nothing here needs to read the
// list, so nothing here should rewrite it.
func bulkUpdateList(raw []byte) (json.RawMessage, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 {
		return nil, usageErr(errors.New(msgBulkUpdateNeedsList))
	}
	for _, item := range list {
		var obj map[string]any
		// A JSON null unmarshals into a map without complaint, and leaves it
		// nil, so the nil is the check.
		if err := json.Unmarshal(item, &obj); err != nil || obj == nil {
			return nil, usageErr(errors.New(msgBulkUpdateNeedsList))
		}
	}
	return json.RawMessage(raw), nil
}
