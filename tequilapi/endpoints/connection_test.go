/*
 * Copyright (C) 2017 The "MysteriumNetwork/node" Authors.
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

package endpoints

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/mysteriumnetwork/node/consumer"
	"github.com/mysteriumnetwork/node/core/connection"
	"github.com/mysteriumnetwork/node/identity"
	"github.com/mysteriumnetwork/node/identity/registry"
	"github.com/mysteriumnetwork/node/market"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type mockConnectionManager struct {
	onConnectReturn       error
	onDisconnectReturn    error
	onStatusReturn        connection.Status
	disconnectCount       int
	requestedConsumerID   identity.Identity
	requestedProvider     identity.Identity
	requestedAccountantID identity.Identity
	requestedServiceType  string
}

func (cm *mockConnectionManager) Connect(consumerID, accountantID identity.Identity, proposal market.ServiceProposal, options connection.ConnectParams) error {
	cm.requestedConsumerID = consumerID
	cm.requestedAccountantID = accountantID
	cm.requestedProvider = identity.FromAddress(proposal.ProviderID)
	cm.requestedServiceType = proposal.ServiceType
	return cm.onConnectReturn
}

func (cm *mockConnectionManager) Status() connection.Status {

	return cm.onStatusReturn
}

func (cm *mockConnectionManager) Disconnect() error {
	cm.disconnectCount++
	return cm.onDisconnectReturn
}

func (cm *mockConnectionManager) Wait() error {
	return nil
}

type StubStatisticsTracker struct {
	duration time.Duration
	stats    consumer.SessionStatistics
}

func (ssk *StubStatisticsTracker) Retrieve() consumer.SessionStatistics {
	return ssk.stats
}

func (ssk *StubStatisticsTracker) GetSessionDuration() time.Duration {
	return ssk.duration
}

func mockRepositoryWithProposal(providerID, serviceType string) *mockProposalRepository {
	sampleProposal := market.ServiceProposal{
		ID:                1,
		ServiceType:       serviceType,
		ServiceDefinition: TestServiceDefinition{},
		ProviderID:        providerID,
	}

	return &mockProposalRepository{
		proposals: []market.ServiceProposal{sampleProposal},
	}
}

func TestAddRoutesForConnectionAddsRoutes(t *testing.T) {
	router := httprouter.New()
	fakeManager := mockConnectionManager{}
	statsKeeper := &StubStatisticsTracker{
		duration: time.Minute,
	}

	mockedProposalProvider := mockRepositoryWithProposal("node1", "noop")
	AddRoutesForConnection(router, &fakeManager, statsKeeper, mockedProposalProvider, mockIdentityRegistryInstance)

	tests := []struct {
		method         string
		path           string
		body           string
		expectedStatus int
		expectedJSON   string
	}{
		{
			http.MethodGet, "/connection", "",
			http.StatusOK, `{"status": ""}`,
		},
		{
			http.MethodPut, "/connection", `{"consumerId": "me", "providerId": "node1", "accountantId":"accountant", "serviceType": "noop"}`,
			http.StatusCreated, `{"status": ""}`,
		},
		{
			http.MethodDelete, "/connection", "",
			http.StatusAccepted, "",
		},
		{
			http.MethodGet, "/connection/statistics", "",
			http.StatusOK, `{
				"bytesSent": 0,
				"bytesReceived": 0,
				"duration": 60
			}`,
		},
	}

	for _, test := range tests {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		router.ServeHTTP(resp, req)
		assert.Equal(t, test.expectedStatus, resp.Code)
		if test.expectedJSON != "" {
			assert.JSONEq(t, test.expectedJSON, resp.Body.String())
		} else {
			assert.Equal(t, "", resp.Body.String())
		}
	}
}

func TestDisconnectingState(t *testing.T) {
	var fakeManager = mockConnectionManager{}
	fakeManager.onStatusReturn = connection.Status{
		State:     connection.Disconnecting,
		SessionID: "",
	}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodGet, "/irrelevant", nil)
	resp := httptest.NewRecorder()

	connEndpoint.Status(resp, req, nil)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(
		t,
		`{
			"status" : "Disconnecting"
		}`,
		resp.Body.String())
}

func TestNotConnectedStateIsReturnedWhenNoConnection(t *testing.T) {
	var fakeManager = mockConnectionManager{}
	fakeManager.onStatusReturn = connection.Status{
		State:     connection.NotConnected,
		SessionID: "",
	}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodGet, "/irrelevant", nil)
	resp := httptest.NewRecorder()

	connEndpoint.Status(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(
		t,
		`{
            "status" : "NotConnected"
        }`,
		resp.Body.String(),
	)
}

func TestStateConnectingIsReturnedWhenIsConnectionInProgress(t *testing.T) {
	var fakeManager = mockConnectionManager{}
	fakeManager.onStatusReturn = connection.Status{
		State: connection.Connecting,
	}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodGet, "/irrelevant", nil)
	resp := httptest.NewRecorder()

	connEndpoint.Status(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(
		t,
		`{
            "status" : "Connecting"
        }`,
		resp.Body.String(),
	)
}

func TestConnectedStateAndSessionIdIsReturnedWhenIsConnected(t *testing.T) {
	var fakeManager = mockConnectionManager{}
	fakeManager.onStatusReturn = connection.Status{
		State:     connection.Connected,
		SessionID: "My-super-session",
	}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodGet, "/irrelevant", nil)
	resp := httptest.NewRecorder()

	connEndpoint.Status(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(
		t,
		`{
			"status" : "Connected",
			"sessionId" : "My-super-session"
		}`,
		resp.Body.String())

}

func TestPutReturns400ErrorIfRequestBodyIsNotJSON(t *testing.T) {
	fakeManager := mockConnectionManager{}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodPut, "/irrelevant", strings.NewReader("a"))
	resp := httptest.NewRecorder()

	connEndpoint.Create(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusBadRequest, resp.Code)

	assert.JSONEq(
		t,
		`{
			"message" : "invalid character 'a' looking for beginning of value"
		}`,
		resp.Body.String())
}

func TestPutReturns422ErrorIfRequestBodyIsMissingFieldValues(t *testing.T) {
	fakeManager := mockConnectionManager{}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodPut, "/irrelevant", strings.NewReader("{}"))
	resp := httptest.NewRecorder()

	connEndpoint.Create(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	assert.JSONEq(
		t,
		`{
			"message" : "validation_error",
			"errors" : {
				"accountantId" : [ {"code" : "required" , "message" : "Field is required" } ],
				"consumerId" : [ { "code" : "required" , "message" : "Field is required" } ],
				"providerId" : [ {"code" : "required" , "message" : "Field is required" } ]
			}
		}`, resp.Body.String())
}

func TestPutWithValidBodyCreatesConnection(t *testing.T) {
	fakeManager := mockConnectionManager{}

	proposalProvider := mockRepositoryWithProposal("required-node", "openvpn")
	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, proposalProvider, mockIdentityRegistryInstance)
	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId" : "accountant"
			}`))
	resp := httptest.NewRecorder()

	connEndpoint.Create(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusCreated, resp.Code)

	assert.Equal(t, identity.FromAddress("my-identity"), fakeManager.requestedConsumerID)
	assert.Equal(t, identity.FromAddress("accountant"), fakeManager.requestedAccountantID)
	assert.Equal(t, identity.FromAddress("required-node"), fakeManager.requestedProvider)
	assert.Equal(t, "openvpn", fakeManager.requestedServiceType)
}

func TestPutUnregisteredIdentityReturnsError(t *testing.T) {
	fakeManager := mockConnectionManager{}

	proposalProvider := mockRepositoryWithProposal("required-node", "openvpn")
	mir := *mockIdentityRegistryInstance
	mir.RegistrationStatus = registry.Unregistered

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, proposalProvider, &mir)
	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId" : "accountant"
			}`))
	resp := httptest.NewRecorder()

	connEndpoint.Create(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusExpectationFailed, resp.Code)
	assert.JSONEq(
		t,
		`{"message":"identity \"my-identity\" is not registered. Please register the identity first"}`,
		resp.Body.String(),
	)
}

func TestPutFailedRegistrationCheckReturnsError(t *testing.T) {
	fakeManager := mockConnectionManager{}

	proposalProvider := mockRepositoryWithProposal("required-node", "openvpn")
	mir := *mockIdentityRegistryInstance
	mir.RegistrationCheckError = errors.New("explosions everywhere")

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, proposalProvider, &mir)
	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId" : "accountant"
			}`))
	resp := httptest.NewRecorder()

	connEndpoint.Create(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.JSONEq(
		t,
		`{"message":"explosions everywhere"}`,
		resp.Body.String(),
	)
}

func TestPutWithServiceTypeOverridesDefault(t *testing.T) {
	fakeManager := mockConnectionManager{}

	mystAPI := mockRepositoryWithProposal("required-node", "noop")
	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, mystAPI, mockIdentityRegistryInstance)
	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId": "accountant",
				"serviceType": "noop"
			}`))
	resp := httptest.NewRecorder()

	connEndpoint.Create(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusCreated, resp.Code)

	assert.Equal(t, identity.FromAddress("required-node"), fakeManager.requestedProvider)
	assert.Equal(t, identity.FromAddress("accountant"), fakeManager.requestedAccountantID)
	assert.Equal(t, identity.FromAddress("required-node"), fakeManager.requestedProvider)
	assert.Equal(t, "noop", fakeManager.requestedServiceType)
}

func TestDeleteCallsDisconnect(t *testing.T) {
	fakeManager := mockConnectionManager{}

	connEndpoint := NewConnectionEndpoint(&fakeManager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(http.MethodDelete, "/irrelevant", nil)
	resp := httptest.NewRecorder()

	connEndpoint.Kill(resp, req, httprouter.Params{})

	assert.Equal(t, http.StatusAccepted, resp.Code)

	assert.Equal(t, fakeManager.disconnectCount, 1)
}

func TestGetStatisticsEndpointReturnsStatistics(t *testing.T) {
	statsKeeper := &StubStatisticsTracker{
		duration: time.Minute,
		stats:    consumer.SessionStatistics{BytesSent: 1, BytesReceived: 2},
	}

	manager := mockConnectionManager{}
	connEndpoint := NewConnectionEndpoint(&manager, statsKeeper, &mockProposalRepository{}, mockIdentityRegistryInstance)

	resp := httptest.NewRecorder()
	connEndpoint.GetStatistics(resp, nil, nil)
	assert.JSONEq(
		t,
		`{
			"bytesSent": 1,
			"bytesReceived": 2,
			"duration": 60
		}`,
		resp.Body.String(),
	)
}

func TestGetStatisticsEndpointReturnsStatisticsWhenSessionIsNotStarted(t *testing.T) {
	statsKeeper := &StubStatisticsTracker{
		stats: consumer.SessionStatistics{BytesSent: 1, BytesReceived: 2},
	}

	manager := mockConnectionManager{}
	connEndpoint := NewConnectionEndpoint(&manager, statsKeeper, &mockProposalRepository{}, mockIdentityRegistryInstance)

	resp := httptest.NewRecorder()
	connEndpoint.GetStatistics(resp, nil, nil)
	assert.JSONEq(
		t,
		`{
			"bytesSent": 1,
			"bytesReceived": 2,
			"duration": 0
		}`,
		resp.Body.String(),
	)
}

func TestEndpointReturnsConflictStatusIfConnectionAlreadyExists(t *testing.T) {
	manager := mockConnectionManager{}
	manager.onConnectReturn = connection.ErrAlreadyExists

	mystAPI := mockRepositoryWithProposal("required-node", "openvpn")
	connectionEndpoint := NewConnectionEndpoint(&manager, nil, mystAPI, mockIdentityRegistryInstance)

	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId" : "accountant"
			}`))
	resp := httptest.NewRecorder()

	connectionEndpoint.Create(resp, req, nil)

	assert.Equal(t, http.StatusConflict, resp.Code)
	assert.JSONEq(
		t,
		`{
			"message" : "connection already exists"
		}`,
		resp.Body.String(),
	)
}

func TestDisconnectReturnsConflictStatusIfConnectionDoesNotExist(t *testing.T) {
	manager := mockConnectionManager{}
	manager.onDisconnectReturn = connection.ErrNoConnection

	connectionEndpoint := NewConnectionEndpoint(&manager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/irrelevant",
		nil,
	)
	resp := httptest.NewRecorder()

	connectionEndpoint.Kill(resp, req, nil)

	assert.Equal(t, http.StatusConflict, resp.Code)
	assert.JSONEq(
		t,
		`{
			"message" : "no connection exists"
		}`,
		resp.Body.String(),
	)
}

func TestConnectReturnsConnectCancelledStatusWhenErrConnectionCancelledIsEncountered(t *testing.T) {
	manager := mockConnectionManager{}
	manager.onConnectReturn = connection.ErrConnectionCancelled

	mockProposalProvider := mockRepositoryWithProposal("required-node", "openvpn")
	connectionEndpoint := NewConnectionEndpoint(&manager, nil, mockProposalProvider, mockIdentityRegistryInstance)
	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId" : "accountant"
			}`))
	resp := httptest.NewRecorder()

	connectionEndpoint.Create(resp, req, nil)

	assert.Equal(t, statusConnectCancelled, resp.Code)
	assert.JSONEq(
		t,
		`{
			"message" : "connection was cancelled"
		}`,
		resp.Body.String(),
	)
}

func TestConnectReturnsErrorIfNoProposals(t *testing.T) {
	manager := mockConnectionManager{}
	manager.onConnectReturn = connection.ErrConnectionCancelled

	connectionEndpoint := NewConnectionEndpoint(&manager, nil, &mockProposalRepository{}, mockIdentityRegistryInstance)
	req := httptest.NewRequest(
		http.MethodPut,
		"/irrelevant",
		strings.NewReader(
			`{
				"consumerId" : "my-identity",
				"providerId" : "required-node",
				"accountantId" : "accountant"
			}`))
	resp := httptest.NewRecorder()

	connectionEndpoint.Create(resp, req, nil)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.JSONEq(
		t,
		`{
			"message" : "provider has no service proposals"
		}`,
		resp.Body.String(),
	)
}

var mockIdentityRegistryInstance = &registry.FakeRegistry{RegistrationStatus: registry.RegisteredConsumer}
