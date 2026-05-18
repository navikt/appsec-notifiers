# appsec-notifiers

A collection of scheduled Slack notifiers maintained by the Application Security team at Nav.

## Notifiers

| Name | Schedule | Description |
|---|---|---|
| `naisteamnotifier` | 1st of month 11:00 | Notifies team owners via Slack DM if their NAIS team is missing from Teamkatalogen |
| `secretsnotifier` | Hourly, Mon–Fri 07:00–17:00 | Notifies teams via Slack about open secret scanning alerts in their GitHub repos |

All notifiers run as NAIS jobs on `prod-gcp`.

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

