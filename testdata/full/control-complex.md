<!-- @doc control-complex -->
<!-- @const env = "production" -->
<!-- @const debug = "true" -->
<!-- @const features = [{ name: "auth", enabled: "true" }, { name: "cache", enabled: "false" }, { name: "logging", enabled: "true" }] -->
<!-- @const servers = ["tokyo", "london", "seoul"] -->

# Control Complex

<!-- @if env == "production" -->
## Production Mode

<!-- @if debug == "true" -->
### Debug Enabled
Warning: Debug is ON in production!
<!-- @endif -->

### Feature Flags
<!-- @for feature in features -->
- {{feature.name}}: {{feature.enabled}}
<!-- @endfor -->

### Servers
<!-- @for server in servers -->
#### {{server}}
Server {{server}} is running.
<!-- @endfor -->
<!-- @endif -->

<!-- @if env == "staging" -->
## Staging Mode
This should NOT appear.
<!-- @endif -->

## Config Summary
Environment: {{env}}
Debug: {{debug}}
