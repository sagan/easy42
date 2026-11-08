import composerize from "composerize";
import * as shlex from "shlex";

export interface DockerRunConversionResult {
  composeYaml: string;
  error?: string;
}

export interface SystemdConversionResult {
  unitContent: string;
  error?: string;
  execStart?: string;
  isAbsolute?: boolean;
}

/**
 * Converts a docker run command string into docker-compose.yaml content using composerize.
 */
export function dockerRunToCompose(cmd: string): DockerRunConversionResult {
  try {
    const trimmed = cmd.trim();
    if (!trimmed) {
      return { composeYaml: "", error: "Please enter a docker run command" };
    }

    // Basic normalization if user omitted 'docker run'
    let commandToParse = trimmed;
    if (!commandToParse.startsWith("docker run") && !commandToParse.startsWith("podman run")) {
      if (commandToParse.startsWith("run ")) {
        commandToParse = "docker " + commandToParse;
      } else {
        commandToParse = "docker run " + commandToParse;
      }
    }

    const result = composerize(commandToParse);
    if (!result || typeof result !== "string") {
      return { composeYaml: "", error: "Could not generate compose file from the given command." };
    }

    return { composeYaml: result };
  } catch (err: any) {
    return {
      composeYaml: "",
      error: err?.message || "Failed to parse docker run command. Please check the syntax.",
    };
  }
}

/**
 * Converts a single command line into a systemd service unit file.
 * If the program is not an absolute path, wraps it with /bin/sh -c "exec ..."
 * with careful quote escaping.
 */
export function cmdlineToSystemd(
  cmd: string,
  options?: { description?: string; user?: string; workingDir?: string; envFile?: string; appName?: string },
): SystemdConversionResult {
  try {
    const trimmed = cmd.trim();
    if (!trimmed) {
      return { unitContent: "", error: "Command line cannot be empty" };
    }

    const tokens = shlex.split(trimmed);
    if (!tokens || tokens.length === 0) {
      return { unitContent: "", error: "No executable or arguments found in command line" };
    }

    const program = tokens[0];
    const isAbsolute = program.startsWith("/");

    let execStart = "";
    if (isAbsolute) {
      // Direct absolute executable
      execStart = shlex.join(tokens);
    } else {
      // Non-absolute path: systemd requires absolute path for ExecStart.
      // We generate: ExecStart=/bin/sh -c "exec ..."
      const shellCmd = "exec " + shlex.join(tokens);
      const escaped = shellCmd.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
      execStart = `/bin/sh -c "${escaped}"`;
    }

    const desc = options?.description?.trim() || `${program} service`;
    const user = options?.user?.trim() || "root";
    const workingDir = options?.workingDir?.trim() || "/var/log";
    const appName = options?.appName?.trim() || "sample";
    const envFile = options?.envFile !== undefined ? options.envFile : `/etc/systemd/system/ezapp-${appName}.env`;

    const unitLines: string[] = [
      "[Unit]",
      `Description=${desc}`,
      "After=network.target",
      "",
      "[Service]",
      "Type=simple",
      `User=${user}`,
      `WorkingDirectory=${workingDir}`,
    ];

    if (envFile) {
      unitLines.push(`EnvironmentFile=${envFile}`);
    }

    unitLines.push(
      `ExecStart=${execStart}`,
      "Restart=always",
      "RestartSec=5",
      "",
      "[Install]",
      "WantedBy=multi-user.target",
      "",
    );

    return {
      unitContent: unitLines.join("\n"),
      execStart,
      isAbsolute,
    };
  } catch (err: any) {
    return {
      unitContent: "",
      error: err?.message || "Failed to parse command line syntax.",
    };
  }
}

/**
 * Ensures that the systemd service unit contains the EnvironmentFile directive
 * for /etc/systemd/system/ezapp-<appName>.env under [Service].
 */
export function ensureSystemdEnvironmentFile(appName: string, content: string): string {
  const cleanName = appName.trim() || "sample";
  const expectedPath = `/etc/systemd/system/ezapp-${cleanName}.env`;
  if (content.includes(expectedPath)) {
    return content;
  }
  const samplePath = "/etc/systemd/system/ezapp-sample.env";
  if (content.includes(samplePath)) {
    return content.replace(samplePath, expectedPath);
  }

  const expectedDirective = `EnvironmentFile=${expectedPath}`;
  const lines = content.split("\n");
  const serviceIdx = lines.findIndex((l) => l.trim().startsWith("[Service]"));
  if (serviceIdx !== -1) {
    const newLines = [...lines.slice(0, serviceIdx + 1), expectedDirective, ...lines.slice(serviceIdx + 1)];
    return newLines.join("\n");
  }

  const trimmed = content.trimEnd();
  if (!trimmed) {
    return `[Service]\n${expectedDirective}\n`;
  }
  return `${trimmed}\n\n[Service]\n${expectedDirective}\n`;
}
