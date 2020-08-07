#! /bin/bash

set -e -o pipefail

# We always use RSA keys since the other types are either insecure or not
# available in FIPS mode

PRIMARY=primary-key
SECONDARY=secondary-key

function gen-key {
    OUTFILE="$1"
    
    rm -f "$OUTFILE" "$OUTFILE.pub"
    ssh-keygen -q -t rsa -b 4096 -f "$OUTFILE" -C '' -N '' 
}

gen-key "$PRIMARY"
gen-key "$SECONDARY"

cat - <<PRIMARY > primary-secret.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: primary-secret
type: Opaque
data:
  primary: $(base64 -w0 < "$PRIMARY")
  primary.pub: $(base64 -w0 < "$PRIMARY.pub")
  secondary.pub: $(base64 -w0 < "$SECONDARY.pub")
PRIMARY

cat - <<SECONDARY > secondary-secret.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: secondary-secret
type: Opaque
data:
  primary.pub: $(base64 -w0 < "$PRIMARY.pub")
  secondary: $(base64 -w0 < "$SECONDARY")
  secondary.pub: $(base64 -w0 < "$SECONDARY.pub")
SECONDARY

rm -f "${PRIMARY}" "${PRIMARY}.pub" "${SECONDARY}" "${SECONDARY}.pub"
