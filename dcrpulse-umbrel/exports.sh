# Host ports for the two agent interfaces. They sit beside the app proxy
# because MCP clients authenticate with a bearer token, not an Umbrel
# session. Both listeners stay off until enabled in the dashboard.
export APP_DCRPULSE_MCP_AGENTS_PORT="8750"
export APP_DCRPULSE_MCP_BRIDGE_PORT="8751"
