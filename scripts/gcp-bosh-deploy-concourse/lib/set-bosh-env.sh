#/bin/bash

: ${BBL_WORKING_DIR:?"Need to set BBL_WORKING_DIR non-empty. Where is your bbl-state.json??"}

export BBL_IAAS=gcp
export BOSH_CLIENT=$(bbl --state-dir $BBL_WORKING_DIR director-username)
export BOSH_CLIENT_SECRET=$(bbl --state-dir $BBL_WORKING_DIR director-password)
export BOSH_CA_CERT=$(bbl --state-dir $BBL_WORKING_DIR director-ca-cert)
export BOSH_ENVIRONMENT=$(bbl --state-dir $BBL_WORKING_DIR director-address)
export BOSH_NON_INTERACTIVE=true
# env | grep BOSH_
