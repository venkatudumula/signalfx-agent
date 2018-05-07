// +build ignore

package neopy

import (
	"github.com/signalfx/golib/datapoint"
	log "github.com/sirupsen/logrus"
)

func handleDatapoints() {
	var dp datapoint.Datapoint
	err = dp.UnmarshalJSON(*msg.Datapoint)
	if err != nil {
		log.WithFields(log.Fields{
			"dpJSON": string(*msg.Datapoint),
			"error":  err,
		}).Error("Could not deserialize datapoint from Python runner")
		continue
	}

	log.WithFields(log.Fields{
		"dp": dp,
	}).Debug("Datapoint received from Python runner")

	ch <- &dp
}
