import type {
  ConnectedAccount,
  ConnectedAccountProvider,
  RepositorySourceProvider,
} from "@/api";

const connectedProviders = new Set<RepositorySourceProvider>([
  "github",
  "slack",
  "devrev",
  "google_drive",
]);

export function isConnectedAccountProvider(
  provider: RepositorySourceProvider,
): provider is ConnectedAccountProvider {
  return connectedProviders.has(provider);
}

export function isUsableConnectedAccount(connection: ConnectedAccount) {
  return connection.status === "active" || connection.status === "demo";
}

export function connectedAccountsForProvider(
  connections: ConnectedAccount[],
  provider: ConnectedAccountProvider,
) {
  return connections.filter((connection) => connection.provider === provider);
}

export function connectedAccountLabel(connection: ConnectedAccount) {
  return (
    connection.display_name ||
    connection.email ||
    connection.external_account_id ||
    `${connection.provider.replaceAll("_", " ")} account`
  );
}

export function connectedAccountContext(connection: ConnectedAccount) {
  return (
    connection.workspace_name ||
    connection.email ||
    connection.external_account_id ||
    connectedAccountLabel(connection)
  );
}
