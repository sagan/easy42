import React, { useState } from "react";
import {
  Box,
  Typography,
  Button,
  Chip,
  Paper,
  TextField,
  Tooltip,
  IconButton,
} from "@mui/material";
import {
  Sparkles,
  Terminal,
  Copy,
  Check,
  RotateCw,
  FolderPlus,
  FileCode,
  ShieldCheck,
  FileJson,
  Info,
} from "lucide-react";

interface CustomTaskHelpProps {
  onRefreshTasks?: () => void;
  isRefreshing?: boolean;
}

export const CustomTaskHelp: React.FC<CustomTaskHelpProps> = ({
  onRefreshTasks,
  isRefreshing,
}) => {
  const [taskGoal, setTaskGoal] = useState<string>(
    "Install and configure chrony NTP client to synchronize time with pool.ntp.org",
  );
  const [copiedPrompt, setCopiedPrompt] = useState<boolean>(false);
  const [copiedQuickstart, setCopiedQuickstart] = useState<boolean>(false);

  const defaultPrompt = `You are an expert Linux systems administrator and shell script developer. I need you to create a custom task for Easy42 Device Helper.

### Task Goal:
${taskGoal.trim() || "[Describe your task goal here, e.g. Install and configure fail2ban]"}

### Easy42 Device Helper Task Architecture:
Easy42 executes tasks over SSH on remote nodes (Debian, Ubuntu, Alpine, OpenWrt, Arch).
For each task, files are placed in: \`~/.config/easy42/tasks/<task_id>/\`
When executed, easy42 creates an isolated temporary directory \`/tmp/.easy42_task_<task_id>_<rand>/\` on target nodes, uploads the task scripts along with \`common.sh\`, and executes them with root privileges.

### Requirements:
1. **run.sh** (REQUIRED, mode 0755):
   - Executable POSIX shell script (\`#!/bin/sh\`).
   - Must safely detect and source \`common.sh\` if present:
     \`\`\`sh
     SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
     if [ -f "$SCRIPT_DIR/common.sh" ]; then
         . "$SCRIPT_DIR/common.sh"
     fi
     \`\`\`
   - You can use standard helper functions from \`common.sh\`:
     - \`$SUDO\`: Contains \`sudo\` if running as non-root user.
     - \`detect_os\`: Populates \`$OS_ID\` (debian, ubuntu, alpine, openwrt, arch).
     - \`detect_init_system\`: Populates \`$INIT_SYSTEM\` (systemd, openrc, procd).
     - \`pkg_update\`: Updates package index for the detected OS.
     - \`pkg_install <package>\`: Installs packages using native package manager (apt-get, apk, opkg, pacman).
   - Must be idempotent where possible.
   - Must output clear, informative messages for stdout/stderr.
   - Must exit with code \`0\` on success, or any non-zero code on failure.

2. **status.sh** (OPTIONAL, mode 0755):
   - Probes the remote node to determine if the task needs execution.
   - Must source \`common.sh\` like \`run.sh\`.
   - Exit codes convention:
     - \`0\`: Task is NOT applied yet / needs execution (UI will show "Needs Execution").
     - \`10\`: Task is already completed / service is properly configured (UI will show "Already Configured").
     - \`20\`: Incompatible or not applicable on this node (UI will show "Not Applicable").
     - Any other code (e.g. \`1\`): Probe error (UI will show "Check Failed").
   - Output a single concise line describing the current state (e.g. "Chrony is installed and running (version 4.3)").

3. **task.json** (OPTIONAL):
   - JSON metadata file:
     \`\`\`json
     {
       "title": "A Human Readable Title",
       "description": "Clear explanation of what this task does on target nodes.",
       "category": "Maintenance",
       "weight": 100
     }
     \`\`\`

Please generate the complete, production-grade files (\`task.json\`, \`status.sh\`, and \`run.sh\`) and the shell commands to create the folder and files under \`~/.config/easy42/tasks/<task_id>/\`.`;

  const quickstartBash = `# 1. Create a custom task folder
mkdir -p ~/.config/easy42/tasks/hello_world

# 2. Write the required run.sh script
cat << 'EOF' > ~/.config/easy42/tasks/hello_world/run.sh
#!/bin/sh
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
[ -f "$SCRIPT_DIR/common.sh" ] && . "$SCRIPT_DIR/common.sh"

echo "Running on node: $(hostname)"
echo "Detected OS: $OS_ID, Init: $INIT_SYSTEM"
echo "System Uptime: $(uptime)"
exit 0
EOF

# 3. Write an optional status.sh probe
cat << 'EOF' > ~/.config/easy42/tasks/hello_world/status.sh
#!/bin/sh
echo "Ready to greet node $(hostname)"
exit 0
EOF

# 4. Make scripts executable
chmod +x ~/.config/easy42/tasks/hello_world/*.sh`;

  const handleCopyPrompt = () => {
    navigator.clipboard.writeText(defaultPrompt);
    setCopiedPrompt(true);
    setTimeout(() => setCopiedPrompt(false), 2000);
  };

  const handleCopyQuickstart = () => {
    navigator.clipboard.writeText(quickstartBash);
    setCopiedQuickstart(true);
    setTimeout(() => setCopiedQuickstart(false), 2000);
  };

  return (
    <Box
      sx={{
        flex: 1,
        overflowY: "auto",
        p: { xs: 2, sm: 3 },
        bgcolor: "#f8fafc",
        display: "flex",
        flexDirection: "column",
        gap: 3,
      }}
    >
      {/* Top Banner */}
      <Paper
        elevation={0}
        sx={{
          p: 3,
          borderRadius: 3,
          border: "1px solid #e2e8f0",
          background: "linear-gradient(135deg, #ffffff 0%, #f1f5f9 100%)",
          display: "flex",
          flexDirection: { xs: "column", sm: "row" },
          alignItems: { xs: "flex-start", sm: "center" },
          justifyContent: "space-between",
          gap: 2,
        }}
      >
        <Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 0.5 }}>
            <Box
              sx={{
                width: 38,
                height: 38,
                borderRadius: 2,
                bgcolor: "primary.main",
                color: "#ffffff",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <FolderPlus size={20} />
            </Box>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0f172a" }}>
              Device Helper Custom Tasks
            </Typography>
            <Chip
              size="small"
              label="Filesystem Driven"
              color="primary"
              variant="outlined"
              sx={{ fontWeight: 700, fontSize: "0.7rem", height: 22 }}
            />
          </Box>
          <Typography variant="body2" sx={{ color: "#475569", maxWidth: 680 }}>
            Define custom automation and maintenance scripts executed across your nodes via SSH.
            Easy42 automatically discovers tasks from your server's local filesystem.
          </Typography>
        </Box>

        {onRefreshTasks && (
          <Button
            variant="outlined"
            size="small"
            onClick={onRefreshTasks}
            disabled={isRefreshing}
            startIcon={
              <RotateCw
                size={14}
                className={isRefreshing ? "animate-spin" : undefined}
              />
            }
            sx={{
              textTransform: "none",
              fontWeight: 600,
              bgcolor: "#ffffff",
              whiteSpace: "nowrap",
            }}
          >
            {isRefreshing ? "Scanning Tasks..." : "Refresh Task List"}
          </Button>
        )}
      </Paper>

      {/* How it Works / Directory Specification Card */}
      <Paper
        elevation={0}
        sx={{
          p: 3,
          borderRadius: 3,
          border: "1px solid #e2e8f0",
          bgcolor: "#ffffff",
        }}
      >
        <Typography variant="subtitle1" sx={{ fontWeight: 700, mb: 1, color: "#0f172a" }}>
          How Custom Tasks Work
        </Typography>
        <Typography variant="body2" sx={{ color: "#475569", mb: 2 }}>
          For safety and simplicity, custom tasks are managed directly in your easy42 data directory:
        </Typography>

        <Box
          sx={{
            bgcolor: "#0f172a",
            color: "#38bdf8",
            p: 2,
            borderRadius: 2,
            fontFamily: "monospace",
            fontSize: "0.85rem",
            mb: 2.5,
            border: "1px solid #1e293b",
          }}
        >
          <Box sx={{ color: "#94a3b8", mb: 0.5 }}># Custom tasks folder location:</Box>
          <Box sx={{ color: "#f8fafc", fontWeight: 600 }}>&lt;easy42-data-dir&gt;/tasks/&lt;task_id&gt;/</Box>
          <Box sx={{ color: "#64748b", fontSize: "0.75rem", mt: 0.5 }}>
            (Default: ~/.config/easy42/tasks/ or overridden by --data-dir)
          </Box>
        </Box>

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", md: "repeat(3, 1fr)" },
            gap: 2,
          }}
        >
          {/* run.sh card */}
          <Box
            sx={{
              p: 2,
              borderRadius: 2,
              border: "1px solid #e2e8f0",
              bgcolor: "#f8fafc",
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              <FileCode size={16} color="#2563eb" />
              <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                run.sh
              </Typography>
              <Chip
                label="Required"
                size="small"
                color="error"
                sx={{ height: 18, fontSize: "0.65rem", fontWeight: 700 }}
              />
            </Box>
            <Typography variant="caption" sx={{ color: "#64748b", display: "block", lineHeight: 1.5 }}>
              Executable shell script that performs the actual installation or configuration.
              Exit with code <strong>0</strong> on success, or non-zero on failure.
            </Typography>
          </Box>

          {/* status.sh card */}
          <Box
            sx={{
              p: 2,
              borderRadius: 2,
              border: "1px solid #e2e8f0",
              bgcolor: "#f8fafc",
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              <ShieldCheck size={16} color="#16a34a" />
              <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                status.sh
              </Typography>
              <Chip
                label="Optional"
                size="small"
                variant="outlined"
                sx={{ height: 18, fontSize: "0.65rem", fontWeight: 600 }}
              />
            </Box>
            <Typography variant="caption" sx={{ color: "#64748b", display: "block", lineHeight: 1.5 }}>
              Checks node state before running. Exit <strong>0</strong> = Needs Execution,{" "}
              <strong>10</strong> = Already Done, <strong>20</strong> = Incompatible. If omitted,
              easy42 marks task as Ready.
            </Typography>
          </Box>

          {/* task.json card */}
          <Box
            sx={{
              p: 2,
              borderRadius: 2,
              border: "1px solid #e2e8f0",
              bgcolor: "#f8fafc",
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              <FileJson size={16} color="#9333ea" />
              <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                task.json
              </Typography>
              <Chip
                label="Optional"
                size="small"
                variant="outlined"
                sx={{ height: 18, fontSize: "0.65rem", fontWeight: 600 }}
              />
            </Box>
            <Typography variant="caption" sx={{ color: "#64748b", display: "block", lineHeight: 1.5 }}>
              Metadata defining title, description, category, and weight. If omitted, easy42
              auto-formats the directory name (e.g. <code>install_bird</code> &rarr; "Install Bird").
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 2, color: "#64748b" }}>
          <Info size={15} />
          <Typography variant="caption">
            <strong>Shared Helpers:</strong> All custom tasks automatically have access to{" "}
            <code>common.sh</code> in their working directory, with helper utilities like{" "}
            <code>detect_os</code>, <code>pkg_install</code>, and <code>$SUDO</code>.
          </Typography>
        </Box>
      </Paper>

      {/* AI Prompt Generator Card */}
      <Paper
        elevation={0}
        sx={{
          p: 3,
          borderRadius: 3,
          border: "1px solid #c7d2fe",
          bgcolor: "#f5f3ff",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Sparkles size={18} color="#7c3aed" />
            <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#4c1d95" }}>
              Ready-to-Use AI Generator Prompt
            </Typography>
          </Box>
          <Button
            variant="contained"
            size="small"
            onClick={handleCopyPrompt}
            startIcon={copiedPrompt ? <Check size={14} /> : <Copy size={14} />}
            sx={{
              textTransform: "none",
              fontWeight: 700,
              bgcolor: copiedPrompt ? "#16a34a" : "#7c3aed",
              "&:hover": {
                bgcolor: copiedPrompt ? "#15803d" : "#6d28d9",
              },
            }}
          >
            {copiedPrompt ? "Copied Prompt!" : "Copy AI Prompt"}
          </Button>
        </Box>

        <Typography variant="body2" sx={{ color: "#5b21b6", mb: 2 }}>
          Describe what you want to achieve on your devices, then copy the prompt into any AI
          (ChatGPT, Claude, Gemini, DeepSeek) to generate production-ready task scripts!
        </Typography>

        <TextField
          label="Your Desired Task Goal"
          fullWidth
          size="small"
          value={taskGoal}
          onChange={(e) => setTaskGoal(e.target.value)}
          placeholder="e.g. Install and configure chrony NTP, or configure Docker and Compose..."
          sx={{
            mb: 2,
            bgcolor: "#ffffff",
            borderRadius: 1.5,
            "& .MuiOutlinedInput-root": {
              borderRadius: 1.5,
            },
          }}
        />

        {/* Prompt Preview Code Box */}
        <Box
          sx={{
            position: "relative",
            bgcolor: "#0f172a",
            color: "#e2e8f0",
            borderRadius: 2,
            p: 2,
            maxHeight: 220,
            overflowY: "auto",
            fontFamily: "monospace",
            fontSize: "0.78rem",
            whiteSpace: "pre-wrap",
            border: "1px solid #334155",
          }}
        >
          {defaultPrompt}
        </Box>
      </Paper>

      {/* 1-Minute Quickstart Card */}
      <Paper
        elevation={0}
        sx={{
          p: 3,
          borderRadius: 3,
          border: "1px solid #e2e8f0",
          bgcolor: "#ffffff",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Terminal size={18} color="#0f172a" />
            <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0f172a" }}>
              Quickstart: Create Your First Custom Task
            </Typography>
          </Box>
          <Tooltip title={copiedQuickstart ? "Copied!" : "Copy bash script"}>
            <IconButton size="small" onClick={handleCopyQuickstart} sx={{ color: "#475569" }}>
              {copiedQuickstart ? <Check size={16} color="#16a34a" /> : <Copy size={16} />}
            </IconButton>
          </Tooltip>
        </Box>

        <Typography variant="body2" sx={{ color: "#475569", mb: 1.5 }}>
          Paste this snippet into your server terminal to create an instant <code>hello_world</code> task:
        </Typography>

        <Box
          sx={{
            bgcolor: "#0f172a",
            color: "#38bdf8",
            borderRadius: 2,
            p: 2,
            fontFamily: "monospace",
            fontSize: "0.8rem",
            whiteSpace: "pre-wrap",
            border: "1px solid #1e293b",
          }}
        >
          {quickstartBash}
        </Box>
      </Paper>
    </Box>
  );
};
