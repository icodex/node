/*
 * Copyright (C) 2018 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package noop

import (
	"errors"
	"testing"
	"time"

	"github.com/mysteriumnetwork/node/core/connection"
	"github.com/mysteriumnetwork/node/core/promise"
	"github.com/mysteriumnetwork/node/identity"
	"github.com/mysteriumnetwork/node/logconfig"
	"github.com/mysteriumnetwork/node/market"
	"github.com/mysteriumnetwork/node/money"
	"github.com/stretchr/testify/assert"
)

var (
	providerID = identity.FromAddress("provider-id")
	proposal   = market.ServiceProposal{
		ProviderID:    providerID.Address,
		PaymentMethod: fakePaymentMethod{},
	}
)

var _ connection.PromiseIssuer = &PromiseIssuer{}

func TestPromiseIssuer_Start_SubscriptionFails(t *testing.T) {
	dialog := &fakeDialog{
		returnError: errors.New("reject subscriptions"),
	}

	capturer := logconfig.NewLogCapturer()
	capturer.Attach()
	defer capturer.Detach()

	issuer := &PromiseIssuer{dialog: dialog, signer: &identity.SignerFake{}}
	err := issuer.Start(proposal)
	defer issuer.Stop()

	assert.EqualError(t, err, "reject subscriptions")
	assert.Len(t, capturer.Messages(), 0)
}

func TestPromiseIssuer_Start_SubscriptionOfBalances(t *testing.T) {
	dialog := &fakeDialog{
		returnReceiveMessage: promise.BalanceMessage{RequestID: 1, Accepted: true, Balance: testToken(1000000000)},
	}

	capturer := logconfig.NewLogCapturer()
	capturer.Attach()
	defer capturer.Detach()

	issuer := &PromiseIssuer{dialog: dialog, signer: &identity.SignerFake{}}
	err := issuer.Start(proposal)
	assert.NoError(t, err)

	logs := capturer.Messages()
	assert.Len(t, logs, 1)
	assert.Contains(t, logs[0], "Promise balance notified: 1000000000TEST")
}

func testToken(amount uint64) money.Money {
	return money.NewMoney(amount, money.Currency("TEST"))
}

type fakePaymentMethod struct{}

func (fpm fakePaymentMethod) GetPrice() money.Money {
	return money.NewMoney(1111111111, money.Currency("FAKE"))
}

func (fpm fakePaymentMethod) GetType() string {
	return "PER_TIME"
}

func (fpm fakePaymentMethod) GetRate() market.PaymentRate {
	return market.PaymentRate{
		PerTime: time.Minute,
	}
}
