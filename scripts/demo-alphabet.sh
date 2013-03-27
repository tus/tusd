#!/bin/bash
#
# This script demonstrates basic interaction with tusd form BASH/curl.
# Can also be used as a simple way to test, or extend to see how it
# responds to edge cases or learn the basic tech.
#

# Constants
SERVICE="localhost:1080"

# Environment
set -e
__FILE__="$(test -L "${0}" && readlink "${0}" || echo "${0}")"
__DIR__="$(cd "$(dirname "${__FILE__}")"; echo $(pwd);)"

# POST requests the upload location
echo -ne "POST '${SERVICE}' \t\t\t\t\t\t\t"
location=$(curl -s \
  --include \
  --request POST \
  --header 'Content-Range: bytes */26' \
${SERVICE}/files |awk -F': ' '/^Location/ {print $2}' |tr -d '\r')
# `tr -d '\r'` is required or location will have one in it ---^
echo "<-- Location: ${location}"


# PUT some data
echo -ne "PUT  '${SERVICE}${location}' \t\t"
status=$(curl -s \
  --include \
  --request PUT \
  --header 'Content-Length: 3' \
  --header 'Content-Range: bytes 0-2/26' \
  --data 'abc' \
${SERVICE}${location} |head -1 |tr -d '\r')
echo "<-- ${status}"

# check that data with HEAD
echo -ne "HEAD '${SERVICE}${location}' \t\t"
has_range=$(curl -s -I -X HEAD ${SERVICE}${location} |awk -F': ' '/^Range/ {print $2}' |tr -d '\r')
echo "<-- Range: ${has_range}"

# get that data with GET
echo -ne "GET  '${SERVICE}${location}' \t\t"
has_content=$(curl -s ${SERVICE}${location})
echo "<-- ${has_content}"


# PUT some data
echo -ne "PUT  '${SERVICE}${location}' \t\t"
status=$(curl -s \
  --include \
  --request PUT \
  --header 'Content-Length: 3' \
  --header 'Content-Range: bytes 23-25/26' \
  --data 'xyz' \
${SERVICE}${location} |head -1 |tr -d '\r')
echo "<-- ${status}"

# check that data with HEAD
echo -ne "HEAD '${SERVICE}${location}' \t\t"
has_range=$(curl -s -I -X HEAD ${SERVICE}${location} |awk -F': ' '/^Range/ {print $2}' |tr -d '\r')
echo "<-- Range: ${has_range}"

# get that data with GET
echo -ne "GET  '${SERVICE}${location}' \t\t"
has_content=$(curl -s ${SERVICE}${location})
echo "<-- ${has_content}"

# get 404 with GET
echo -ne "GET  '${SERVICE}${location}a' \t\t"
has_content=$(curl -s ${SERVICE}${location})
echo "<-- ${has_content}"

