// Copyright 2019 Sumukha PK
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package routing

import (
	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/common/basecomponent"
)

const (
	// TYPESUM sum type
	TYPESUM = iota
	// BODYDEVICEKEY device key body
	BODYDEVICEKEY
	// BODYONETIMEKEY one time key
	BODYONETIMEKEY
	// ONETIMEKEYSTRING key string
	ONETIMEKEYSTRING
	// ONETIMEKEYOBJECT key object
	ONETIMEKEYOBJECT
)

// ONETIMEKEYSTR stands for storage string property
const ONETIMEKEYSTR = "one_time_key"

// DEVICEKEYSTR stands for storage string property
const DEVICEKEYSTR = "device_key"

// KeyNotifier kafka notifier
type KeyNotifier struct {
	base *basecomponent.BaseDendrite
	ch   sarama.AsyncProducer
}

var keyProducer = &KeyNotifier{}

// ClearUnused when web client sign out, a clean should be processed, cause all keys would never been used from then on.
// todo: complete this function and invoke through sign out extension or some scenarios else those matter
func ClearUnused() {}

// InitNotifier initialize kafka notifier
func InitNotifier(base *basecomponent.BaseDendrite) {
	keyProducer.base = base
	pro, _ := sarama.NewAsyncProducer(base.Cfg.Kafka.Addresses, nil)
	keyProducer.ch = pro
}
