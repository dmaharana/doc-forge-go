# Project-Scoped Confluence Integration and Enforced Atlassian SSO

We replaced global Atlassian OAuth settings with project-scoped Confluence target configuration and enforced Atlassian SSO login for all users. The Confluence target (space and parent page hierarchy) is configured directly inside each project by the project owner, while document publication to Confluence uses the exporting user's authenticated Atlassian session to verify space access and guarantee exact author attribution in Confluence audit trails.
