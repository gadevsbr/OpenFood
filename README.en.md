# OpenFood · Restaurant point of sale

**From the first order to cash closing, on your own computer.**

OpenFood is an open-source, local-first restaurant POS and management system for Windows and Linux. Menu configuration, tables, tabs, kitchen workflow and administration share a local PostgreSQL database. No mandatory central server or commercial subscription. The application interface is currently in Portuguese.

[![Experimental release](https://img.shields.io/github/v/release/gadevsbr/OpenFood?include_prereleases&label=alpha)](https://github.com/gadevsbr/OpenFood/releases)
[![Build and tests](https://github.com/gadevsbr/OpenFood/actions/workflows/release.yml/badge.svg)](https://github.com/gadevsbr/OpenFood/actions/runs/38320810285)
[![AGPL-3.0 license](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

[**Download for Windows**](https://github.com/gadevsbr/OpenFood/releases/download/v0.12.0-alpha.1/OpenFood-0.12.0-alpha.1-windows-x64-setup.exe) · [Português](README.md) · [Installation guide (Portuguese)](docs/installation.md) · [Roadmap (Portuguese)](project-memory/roadmap.md)

![OpenFood restaurant POS with categories, menu and order panel](docs/evidence/restaurant-ui/pos-desktop.png)

> **Experimental alpha · 0.12.0-alpha.1.** Browser/PostgreSQL journeys and Windows/Linux CI have passed within the documented scope. Clean-machine installation, physical peripherals and fiscal operation still have open validation gates. Screenshots use synthetic QA data; this overview shows the visual direction introduced in version 0.9.

## Features

- Separate **PDV (POS) / ADM** workspaces, with a required cash opening before sales.
- Restaurant categories, local photos, sizes/variants, add-on limits, item notes and availability schedules.
- Up to four pizza flavors, weighted portions and highest-price or weighted-average pricing; common size adjustments and add-ons. [Pizza guide (Portuguese)](docs/pizza.md).
- Configurable combo groups, fixed price with adjustments, component sum or highest-component pricing.
- Persistent tables/tabs, preparation rounds, kitchen queue and non-fiscal receipts.
- Manual partial/split payments, change, manager discounts, cash withdrawals/deposits and cash closing with discrepancies.
- Optional customer identification, store-scoped history, versioned consent texts, export and audited manual removal of identification.
- Organization/store isolation, role permissions, transactional stock and recorded prices in integer cents.
- Embedded PostgreSQL and backup/restore UI on Windows; Docker Compose and command-line backup on Linux.

## Installation

### Windows 10/11 x64

Download the [0.12 installer](https://github.com/gadevsbr/OpenFood/releases/download/v0.12.0-alpha.1/OpenFood-0.12.0-alpha.1-windows-x64-setup.exe) and verify [SHA256SUMS](https://github.com/gadevsbr/OpenFood/releases/download/v0.12.0-alpha.1/SHA256SUMS). Install per user, open OpenFood from the Start menu, then create your store, email and a password of at least 12 characters. There are no default credentials. Go, Node and Docker are not required.

### Linux with Docker Compose

```sh
git clone https://github.com/gadevsbr/OpenFood.git
cd OpenFood
cp .env.example .env
```

Set `POSTGRES_PASSWORD` and `SETUP_TOKEN` in `.env` to two different random 32-byte hexadecimal values. Then:

```sh
docker compose up --build -d
curl http://127.0.0.1:18880/readyz
```

Open **http://127.0.0.1:18880** and supply the setup token. The `database` volume persists data. Follow the [installation and backup guide](docs/installation.md) before updating; never remove the database volume to update the app.

## Current limits

Supported networking is loopback only. The computer must remain running. Cash, card and Pix payments are manual records; no bank charges or external refunds are executed. Integrated Pix, TEF, fiscal invoicing, WhatsApp, iFood and AI are not connected. Sales by weight, fees, automatic retention and handling removed identification after restoring old backups remain on the roadmap. Existing privacy controls do not constitute legal-compliance certification.

Fonts, icons and UI scripts are embedded locally; there is no mandatory telemetry. Optional external providers may charge their own fees.

## Development and evidence

Built with **Go 1.27.2, PostgreSQL 17 and an embedded web interface**. Motion mini animations respect reduced-motion preferences. Read [contribution guidelines](CONTRIBUTING.md), [development and QA](docs/development.md) and [security policy](SECURITY.md). Project guides are currently in Portuguese.

[Release 0.12](https://github.com/gadevsbr/OpenFood/releases/tag/v0.12.0-alpha.1) includes Windows/Linux packages, QA evidence and SHA-256 manifests. See [the CI run](https://github.com/gadevsbr/OpenFood/actions/runs/38320810285) and [validation scope](project-memory/pizza-stage.md).

[Report a bug or suggest a feature](https://github.com/gadevsbr/OpenFood/issues/new/choose) using synthetic data and reproducible steps.

## License

[AGPL-3.0-only](LICENSE).
