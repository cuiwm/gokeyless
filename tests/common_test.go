package tests

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/gokeyless"
	"github.com/cloudflare/gokeyless/client"
	"github.com/cloudflare/gokeyless/server"
)

const (
	serverCert   = "testdata/server.pem"
	serverKey    = "testdata/server-key.pem"
	keylessCA    = "testdata/ca.pem"
	serverAddr   = "localhost:3407"
	rsaPrivKey   = "testdata/rsa.key"
	ecdsaPrivKey = "testdata/ecdsa.key"

	clientCert  = "testdata/client.pem"
	clientKey   = "testdata/client-key.pem"
	keyserverCA = "testdata/ca.pem"
	rsaPubKey   = "testdata/rsa.pubkey"
	ecdsaPubKey = "testdata/ecdsa.pubkey"
)

var (
	s        *server.Server
	c        *client.Client
	rsaKey   *client.Decrypter
	ecdsaKey *client.PrivateKey
	remote   client.Remote
)

// Set up compatible server and client for use by tests.
func init() {
	var err error
	var pemBytes []byte
	var p *pem.Block
	var priv crypto.Signer
	var pub crypto.PublicKey

	log.Level = log.LevelFatal

	s, err = server.NewServerFromFile(serverCert, serverKey, keylessCA, serverAddr, "")
	if err != nil {
		log.Fatal(err)
	}

	keys := server.NewDefaultKeystore()
	s.Keys = keys

	// Import RSA private key into server's keystore.
	if pemBytes, err = ioutil.ReadFile(rsaPrivKey); err != nil {
		log.Fatal(err)
	}
	p, _ = pem.Decode(pemBytes)
	if priv, err = x509.ParsePKCS1PrivateKey(p.Bytes); err != nil {
		log.Fatal(err)
	}
	if err = keys.Add(nil, priv); err != nil {
		log.Fatal(err)
	}

	// Import ECDSA private key into server's keystore.
	if pemBytes, err = ioutil.ReadFile(ecdsaPrivKey); err != nil {
		log.Fatal(err)
	}
	p, _ = pem.Decode(pemBytes)
	if priv, err = x509.ParseECPrivateKey(p.Bytes); err != nil {
		log.Fatal(err)
	}
	if err = keys.Add(nil, priv); err != nil {
		log.Fatal(err)
	}

	// Create a dummy GetCertificate function which reads a static cert from disk.
	s.GetCertificate = func(op *gokeyless.Operation) ([]byte, error) {
		return ioutil.ReadFile(tlsChain)
	}

	listening := make(chan bool)
	go func() {
		listening <- true
		if err := s.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	<-listening

	if c, err = client.NewClientFromFile(clientCert, clientKey, keyserverCA); err != nil {
		log.Fatal(err)
	}

	remote, err = c.LookupServer(serverAddr)
	if err != nil {
		log.Fatal(err)
	}

	if pemBytes, err = ioutil.ReadFile(rsaPubKey); err != nil {
		log.Fatal(err)
	}
	p, _ = pem.Decode(pemBytes)
	if pub, err = x509.ParsePKIXPublicKey(p.Bytes); err != nil {
		log.Fatal(err)
	}
	var privKey crypto.Signer
	var ok bool
	if privKey, err = c.RegisterPublicKey(serverAddr, pub); err != nil {
		log.Fatal(err)
	}

	rsaKey, ok = privKey.(*client.Decrypter)
	if !ok {
		log.Fatal("bad RSA key registration")
	}

	if pemBytes, err = ioutil.ReadFile(ecdsaPubKey); err != nil {
		log.Fatal(err)
	}
	p, _ = pem.Decode(pemBytes)
	if pub, err = x509.ParsePKIXPublicKey(p.Bytes); err != nil {
		log.Fatal(err)
	}

	if privKey, err = c.RegisterPublicKey(serverAddr, pub); err != nil {
		log.Fatal(err)
	}

	ecdsaKey, ok = privKey.(*client.PrivateKey)
	if !ok {
		log.Fatal("bad ECDSA key registration")
	}
}
