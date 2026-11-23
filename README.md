# appsec-notifiers

A collection of Slack notifiers maintained by the Application Security team at Nav: scheduled jobs and a delivery API.

## Notifiers

| Name | Schedule | Description |
|---|---|---|
| `naisteamnotifier` | 1st of month 11:00 | Notifies team owners via Slack DM if their NAIS team is missing from Teamkatalogen |
| `secretsnotifier` | Hourly, Mon–Fri 07:00–17:00 | Notifies teams via Slack about open secret scanning alerts in their GitHub repos |

All notifiers run as NAIS jobs on `prod-gcp`.

## Delivery API

`deliveryapi` (Nais app `appsec-notifiers`) is a stateless REST API. It renders a vulnerability digest as a Norwegian Slack message and posts it to a channel. It stores nothing. The contract is [`api/openapi.yaml`](api/openapi.yaml).

- Requests need an Entra ID client credentials token. The token is validated through Nais Texas introspection. Who may call the API is set by the access policy in [`.nais/deliveryapi.yaml`](.nais/deliveryapi.yaml).
- Messages are posted as a Slack bot with the scopes `chat:write`, `channels:read`, `groups:read`, `channels:history` and `groups:history`. The bot must be invited to the channel. Externally shared channels are rejected, and there is no fallback channel.
- Each digest carries its `deliveryId` in Slack message metadata. Before posting, the API searches the last 48 hours of channel history for that ID and returns the existing message if it finds one. If the search fails, nothing is posted and the request fails with a retryable error.
- Slack calls are never retried. Rate limits are returned as `429` with `Retry-After`.

| Variable | Required | Description |
|---|---|---|
| `TPT_SLACK_BOT_TOKEN` | yes | Slack bot token |
| `NAIS_TOKEN_INTROSPECTION_ENDPOINT` | yes | Texas introspection endpoint (injected by Nais) |
| `SLACK_API_URL` | no | Slack Web API base URL; override to use WireMock locally |

## Integrations

- **GitHub API** — fetches repository and security alert data for the `navikt` org
- **NAIS Console** — resolves repository ownership and team Slack channels
- **Teamkatalogen** — cross-references NAIS teams against the official team catalog
- **Slack API** — delivers notifications to team channels or via direct messages

## License
[MIT](LICENSE).

## Contact

This project is maintained by [@appsec](https://github.com/orgs/navikt/teams/appsec).

Questions and/or feature requests? Please create an [issue](https://github.com/navikt/appsec-notifiers/issues).

