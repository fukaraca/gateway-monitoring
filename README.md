# Gateway Monitoring 

Gateway Monitor is a small, config-file driven Linux service that reads /proc (_meminfo, loadavg, net/dev_) to collect CPU, 
memory and network interface stats, verifies connectivity by pinging targets and resolving DNS (_A/AAAA/CNAME/MX/TXT_), and 
exposes basic Prometheus-style metrics for scraping. 

The repo includes a sample config and a systemd service file for easy deployment. Tests aim for >80% coverage.

This is for proving the skill set, no business value.

# Run
```
make install-deps
make run
```

# Installation
installs by the rules packaging/systemd/gateway-monitor.service

`make install-package `

# Uninstallation

`make uninstall-package`

# Testing
Requires Docker

`make test`