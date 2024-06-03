import { QueryClient } from "@tanstack/svelte-query";
import { get } from "svelte/store";
import {
  V1ConnectorDriver,
  getRuntimeServiceGetFileQueryKey,
  runtimeServiceGetFile,
} from "../../runtime-client";
import { runtime } from "../../runtime-client/runtime-store";

export function compileConnectorYAML(
  connector: V1ConnectorDriver,
  formValues: Record<string, unknown>,
) {
  // Add instructions to the top of the file
  const topOfFile = `# Connector YAML
# Reference documentation: https://docs.rilldata.com/reference/project-files/connectors
  
type: connector
driver: ${connector.name}`;

  // Get the secret keys
  const secretKeys =
    connector.sourceProperties
      ?.filter((property) => property.secret)
      .map((property) => property.key) || [];

  // Compile key value pairs
  const compiledKeyValues = Object.entries(formValues)
    .map(([key, value]) =>
      secretKeys.includes(key)
        ? `${key}: "{{ .vars.${makeDotEnvConnectorKey(
            connector.name as string,
            key,
          )} }}"`
        : `${key}: "${value}"`,
    )
    .join("\n");

  // Return the compiled YAML
  return `${topOfFile}\n\n` + compiledKeyValues;
}

export async function updateDotEnvWithSecrets(
  queryClient: QueryClient,
  connector: V1ConnectorDriver,
  formValues: Record<string, string>,
): Promise<string> {
  const instanceId = get(runtime).instanceId;

  // Get the existing .env file
  let blob: string;
  try {
    const file = await queryClient.fetchQuery({
      queryKey: getRuntimeServiceGetFileQueryKey(instanceId, { path: ".env" }),
      queryFn: () => runtimeServiceGetFile(instanceId, { path: ".env" }),
    });
    blob = file.blob || "";
  } catch (error) {
    // Handle the case where the .env file does not exist
    if (error?.response?.data?.message?.includes("no such file")) {
      blob = "";
    } else {
      throw error;
    }
  }

  // Get the secret keys
  const secretKeys = connector.sourceProperties
    ?.filter((property) => property.secret)
    .map((property) => property.key);

  // In reality, all connectors have secret keys, but this is a safeguard
  if (!secretKeys) {
    return blob;
  }

  // Update the blob with the new secrets
  secretKeys.forEach((key) => {
    if (!key) {
      return;
    }

    const connectorSecretKey = makeDotEnvConnectorKey(
      connector.name as string,
      key,
    );
    const secretValue = formValues[key];
    blob = replaceOrAddEnvVariable(blob, connectorSecretKey, secretValue);
  });

  return blob;
}

export function replaceOrAddEnvVariable(
  existingEnvBlob: string,
  key: string,
  newValue: string,
): string {
  const lines = existingEnvBlob.split("\n");
  let keyFound = false;

  const updatedLines = lines.map((line) => {
    if (line.startsWith(`${key}=`)) {
      keyFound = true;
      return `${key}=${newValue}`;
    }
    return line;
  });

  if (!keyFound) {
    updatedLines.push(`${key}=${newValue}`);
  }

  const newBlob = updatedLines
    .filter((line, index) => !(line === "" && index === 0))
    .join("\n")
    .trim();

  return newBlob;
}

export function makeDotEnvConnectorKey(connectorName: string, key: string) {
  // Note: The connector name, not driver, is used. This enables configuring multiple connectors that use the same driver.
  return `connector.${connectorName}.${key}`;
}

export async function updateRillYAMLWithOlapConnector(
  queryClient: QueryClient,
  newConnector: string,
): Promise<string> {
  // Get the existing rill.yaml file
  const instanceId = get(runtime).instanceId;
  const file = await queryClient.fetchQuery({
    queryKey: getRuntimeServiceGetFileQueryKey(instanceId, {
      path: "rill.yaml",
    }),
    queryFn: () => runtimeServiceGetFile(instanceId, { path: "rill.yaml" }),
  });
  const blob = file.blob || "";

  // Update the blob with the new OLAP connector
  return replaceOlapConnectorInYAML(blob, newConnector);
}

/**
 * Update the `olap_connector` key in a YAML file.
 * This function uses a regex approach to preserve comments and formatting.
 */
export function replaceOlapConnectorInYAML(
  blob: string,
  newConnector: string,
): string {
  const olapConnectorRegex = /^olap_connector: .+$/m;

  if (olapConnectorRegex.test(blob)) {
    return blob.replace(olapConnectorRegex, `olap_connector: ${newConnector}`);
  } else {
    return `${blob}${blob !== "" ? "\n" : ""}olap_connector: ${newConnector}\n`;
  }
}
