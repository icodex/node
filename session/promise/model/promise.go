/*
 * Copyright (C) 2019 The "MysteriumNetwork/node" Authors.
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

package model

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mysteriumnetwork/node/identity"
)

// ExtraData represents the extra data
type ExtraData interface {
	Hash() []byte
}

const receiverPrefix = "Receiver prefix:"
const emptyExtra = "emptyextra"

// EmptyExtra represents an empty extra data
type EmptyExtra struct {
}

// Hash returns the hash representation of the data
func (EmptyExtra) Hash() []byte {
	return crypto.Keccak256([]byte(emptyExtra))
}

var _ ExtraData = EmptyExtra{}

// Promise holds all the information related to a promise
type Promise struct {
	Extra    ExtraData
	Receiver common.Address
	SeqNo    uint64
	Amount   uint64
}

const issuerPrefix = "Issuer prefix:"

// Bytes gets a byte representation of the promise
func (p *Promise) Bytes() []byte {
	slices := [][]byte{
		p.Extra.Hash(),
		p.Receiver.Bytes(),
		abi.U256(big.NewInt(0).SetUint64(p.SeqNo)),
		abi.U256(big.NewInt(0).SetUint64(p.Amount)),
	}
	var res []byte
	for _, slice := range slices {
		res = append(res, slice...)
	}
	return res
}

// IssuedPromise represents the signed promise
type IssuedPromise struct {
	Promise
	IssuerSignature []byte
}

// Bytes returns a byte representation of the issued promise
func (ip *IssuedPromise) Bytes() []byte {
	return append([]byte(issuerPrefix), ip.Promise.Bytes()...)
}

// IssuerAddress recovers and returns the issuer address
func (ip *IssuedPromise) IssuerAddress() (common.Address, error) {
	publicKey, err := crypto.Ecrecover(crypto.Keccak256(ip.Bytes()), ip.IssuerSignature)
	if err != nil {
		return common.Address{}, err
	}
	pubKey, err := crypto.UnmarshalPubkey(publicKey)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pubKey), nil
}

// ReceivedPromise represents a promise received by the provider
type ReceivedPromise struct {
	IssuedPromise
	ReceiverSignature []byte
}

// SignByPayer allows the payer to sign the promise
func SignByPayer(promise *Promise, payer identity.Signer) (*IssuedPromise, error) {
	signature, err := payer.Sign(append([]byte(issuerPrefix), promise.Bytes()...))
	if err != nil {
		return nil, err
	}

	return &IssuedPromise{
		*promise,
		signature.Bytes(),
	}, nil
}

// SignByReceiver allows the receiver to sign the promise
func SignByReceiver(promise *IssuedPromise, receiver identity.Signer) (*ReceivedPromise, error) {
	payerAddr, err := promise.IssuerAddress()
	if err != nil {
		return nil, err
	}
	sig, err := receiver.Sign(append(append([]byte(receiverPrefix), crypto.Keccak256(promise.Bytes())...), payerAddr.Bytes()...))
	return &ReceivedPromise{
		*promise,
		sig.Bytes(),
	}, err
}
