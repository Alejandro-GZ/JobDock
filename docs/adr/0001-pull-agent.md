# ADR 0001: Pull-based trusted agents

Status: accepted.

Agents initiate authenticated HTTPS requests to one server port and access only their local Docker socket. This avoids remote Docker TCP exposure and inbound worker firewall rules. The official v0.1 agent runs as a hardened but trusted container; native installation remains a future option.

