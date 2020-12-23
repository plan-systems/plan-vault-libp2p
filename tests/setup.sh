#!/bin/bash
set -eu

SRC_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

gen_config() {
    i=$1
    DIR="${SRC_DIR}/vault${i}"
    mkdir -p ${DIR}/{etc,var}

    # generate a self-signed cert for the vault
    openssl req -x509 -sha256 -nodes -days 30 -newkey rsa:4096 -extensions 'v3_req' \
            -subj "/CN=localhost" \
            -keyout "${DIR}/etc/server.key" \
            -out "${DIR}/etc/server.crt" \
            -extensions san \
            -config <( \
                       echo '[req]'; \
                       echo 'distinguished_name=req'; \
                       echo '[san]'; \
                       echo 'subjectAltName=IP.1:127.0.0.1')

    # generate a unique configuration for the vault
    args=$(printf \
           '.keyring_dir = "%s" |
            .store.data_dir = "%s" |
            .server.port = %s |
            .server.tls_cert_file = "%s" |
            .server.tls_key_file = "%s" |
            .p2p.mdns = false |
            .p2p.tcp_port = %s |
            .p2p.quic_port = %s |
            .debug.port = %s
            ' \
                "${DIR}/etc" \
                "${DIR}/var" \
                "509$i" \
                "${DIR}/etc/server.crt" \
                "${DIR}/etc/server.key" \
                "905$i" \
                "905$i" \
                "906$i" \
                )

    ${SRC_DIR}/../bin/vault -print | jq "$args" > "${DIR}/etc/vault.json"

    # make a matching client
    CLIENT_DIR="${SRC_DIR}/client${i}"
    mkdir -p "${CLIENT_DIR}/etc"
    args=$(printf \
           '.keyring_dir = "%s" |
           del(.store) |
           .server.port = %s |
           .server.tls_cert_file = "%s" |
           del(.server.tls_key_file) |
           del(.p2p.mdns) |
           del(.p2p.tcp_port) |
           del(.p2p.tcp_addr) |
           del(.p2p.quic_port) |
           del(.p2p.quic_addr) |
           del(.debug)
           ' \
               "${CLIENT_DIR}/etc" \
               "509$i" \
               "${DIR}/etc/server.crt" \
               )
    ${SRC_DIR}/../bin/vault -print | jq "$args" > "${CLIENT_DIR}/etc/vault.json"

}

gen_config 1
gen_config 2
