1. Adjust the agent to have predefined commands such that scripts on the VM become redundant in favour of explicit agent code.
2. Staccato will handle deployments, allowing branches to be deployed as either Production or Staging instances
3. Each deployment should require and Odoo version, an addons repo and a branch, an optional tag for fix odoo-env version and an option commit for repo commit
4. Deployments should be based on standardised docker-compose templates
5. The deployment process is pull odoo-env image, pull odoo repo into odoo folder, pull addons folder into repos folder, pip requirements, run odoo, update all modules
6. We should add an on commit strategy for the branch, either do nothing or update
7. We should be able to create a backup, this should also create an anonymised dump
7. A new deployment should create a database dump if none exists, this can be saved on the VM
8. We should allow instances to be restored from backup
9. Deployment of a staging branch from a backup should use an anonymised dump
10. A redeployment should leave the current deployment intact and active until successfully built as a new environment
11. An agent bundle should be generated via the platform web page, this should create the NKEYs and allow env variables to be set, a git token for pulling repos and packages should be added to the bundle.
12. the UI should instruct the user how to use the bundle