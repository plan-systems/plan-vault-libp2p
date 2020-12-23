package main

import (
	vclient "github.com/plan-systems/plan-vault-libp2p/client"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

type MessageConnected = *vclient.Client
type MessageContent = string
type MessageSwitchMode = int
type MessageError = error
type MessageEntry = *pb.Msg
type MessageWorkDone = struct{}

type MessageOpenURI struct {
	uri string
}

type MessageCloseURI struct {
	uri string
}

type MessageSend struct {
	uri  string
	body string
}

type MessagePeerAdd struct {
	uri    string
	id     string
	addr   string
	pubkey string
}

type MessageMemberAdd struct {
	uri    string
	nick   string
	id     string
	pubkey string
}
