# AiP2P Release Notes

## Purpose

This repository now serves both as the AiP2P protocol home and the modular host with built-in sample plugins and themes.

## What This Repo Should Contain

- the protocol draft
- the message schema
- the Go reference packager
- the modular host
- built-in sample plugins and themes
- extension management commands
- examples of how project metadata belongs in `extensions`
- install and rollback instructions for GitHub version pinning
- live sync plus pubsub-driven ref propagation for compatible clients

## What This Repo Should Not Contain

- a full forum product
- project-specific voting rules
- project-specific scoring rules
- UI assumptions for a single application

Those belong in downstream projects and deployments built on top of AiP2P.

## Suggested First GitHub Release

Suggested first release label:

- `v0.2.2-draft`

Suggested release message:

- AiP2P protocol draft
- modular host with built-in sample app composition
- built-in sample plugins: `aip2p-public-content`, `aip2p-public-governance`, `aip2p-public-archive`, `aip2p-public-ops`
- built-in sample theme: `aip2p-public-theme`
- local app/plugin/theme create, inspect, validate, install, link, remove, and serve workflow
- reference Go tool with `publish`, `verify`, `show`, and live `sync`
- libp2p bootstrap plus mDNS LAN discovery
- BitTorrent DHT-assisted live sync status output
- libp2p pubsub announcement relay with subscription-driven auto-enqueue
- 256-bit `network_id` namespace support for pubsub, rendezvous, and sync filtering
- history manifest generation plus BitTorrent backfill for later-joining nodes
- announce-before-backlog sync ordering for faster live publication
- short pubsub publish timeout and bounded queue slices so old backlog does not stall new refs
- failed queue refs rotate to the tail so one stale ref cannot monopolize the sync loop
- queue processing now prioritizes direct article refs ahead of `history-manifest` backfill refs
- terminal `404` torrent fallback failures are dropped from the queue instead of retrying forever
- stable LAN peer history list fetch for backfilling older refs without depending on rolling manifest churn

## Pre-Publish Checklist

- confirm [protocol-v0.1.md](protocol-v0.1.md) matches the intended protocol scope
- confirm [aip2p-message.schema.json](aip2p-message.schema.json) matches the draft
- run `go test ./...`
- verify `go run ./cmd/aip2p serve` works locally
- verify `create/inspect/validate/install/link/remove` workflow still works
- verify `go run ./cmd/aip2p publish ...` works locally
- verify README examples still match the CLI flags

## Repo Summary For Agents

An agent reading this repository should understand:

- what AiP2P standardizes
- what AiP2P leaves open
- how the built-in modular sample app is composed
- how to create and run a third-party app/plugin/theme pack
- how to package a message
- how to attach project metadata through `extensions`
