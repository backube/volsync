#! /bin/bash

set -e -o pipefail

# We always use RSA keys since the other types are either insecure or not
# available in FIPS mode

SOURCE=source-key
DESTINATION=destination-key

function gen-key {
    OUTFILE="$1"
    
    rm -f "$OUTFILE" "$OUTFILE.pub"
    ssh-keygen -q -t rsa -b 4096 -f "$OUTFILE" -C '' -N '' 
}

gen-key "$SOURCE"
gen-key "$DESTINATION"

cat - <<SOURCE > source-secret.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: source-secret
type: Opaque
data:
  source: $(base64 -w0 < "$SOURCE")
  source.pub: $(base64 -w0 < "$SOURCE.pub")
  destination.pub: $(base64 -w0 < "$DESTINATION.pub")
SOURCE

cat - <<DESTINATION > destination-secret.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: destination-secret
type: Opaque
data:
  source.pub: $(base64 -w0 < "$SOURCE.pub")
  destination: $(base64 -w0 < "$DESTINATION")
  destination.pub: $(base64 -w0 < "$DESTINATION.pub")
DESTINATION

rm -f "${SOURCE}" "${SOURCE}.pub" "${DESTINATION}" "${DESTINATION}.pub"
