#!/bin/sh

chown tusd:tusd /srv/tusd-data
chown tusd:tusd /srv/tusd-hooks
/go/bin/tusd -dir /srv/tusd-dat --hooks-dir /srv/tusd-hooks