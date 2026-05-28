# Staccato Agent Installation: asdn

1. Unzip this bundle on your target VM.
2. Ensure you have Docker and Docker Compose installed.
3. Run the agent:
   chmod +x staccato-agent
   export $(cat agent.env | xargs)
   ./staccato-agent

4. The agent will heartbeat to the platform. 
5. Find it in the UI and click "Activate" to begin managing environments.
