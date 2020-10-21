#!/usr/bin/env bash

set -e

## debug if desired
if [[ -n "${DEBUG}" ]]; then
    set -x
fi

## avoid noisy shellcheck warnings
CHART_PATH="./chart"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-dummy/dummy}"
RELEASE_VERSION="${RELEASE_VERSION:-0.0.0}"
RELEASE_USER_TOKEN="${RELEASE_USER_TOKEN:-dummy}"
GITHUB_USER="${GITHUB_USER:-dummy}"
CHARTS_REPOSITORY="${CHARTS_REPOSITORY:-dummy/dummy}"

## temp working vars
TIMESTAMP="$(date +%s )"
TMP_DIR="/tmp/${TIMESTAMP}"

## set up Git-User
git config --global user.name "${RELEASE_USER_NAME}"
git config --global user.email "${RELEASE_USER_EMAIL}"

## temporary clone git repository
git clone "https://github.com/${GITHUB_REPOSITORY}" "${TMP_DIR}"
cd "${TMP_DIR}"

## replace appVersion
sed -i "s#appVersion:.*#appVersion: v${RELEASE_VERSION}#g" "${CHART_PATH}/Chart.yaml"

## replace chart version with current tag without 'v'-prefix
sed -i "s#version:.*#version: ${RELEASE_VERSION}#g" "${CHART_PATH}/Chart.yaml"

## useful for debugging purposes
git status

## Add new remote with credentials baked in url
git remote add publisher "https://${GITHUB_USER}:${RELEASE_USER_TOKEN}@github.com/${GIT_REPOSITORY}"

## add and commit changed file
git add -A

## useful for debugging purposes
git status

## stage changes
git commit -m "Bump appVersion to 'v${RELEASE_VERSION}'"

## rebase
git pull --rebase publisher master

if [[ "${1}" == "publish" ]]; then

    ## publish changes
    git push publisher master

    ## trigger helm-charts reload
    curl -X POST 'https://api.github.com/repos/${CHARTS_REPOSITORY}/dispatches' -u "${RELEASE_USER}:${RELEASE_USER_TOKEN}" -d '{"event_type": "updateCharts"}'

fi

exit 0