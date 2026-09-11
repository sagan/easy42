import React from "react";
import {
  Box,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Switch,
  Typography,
} from "@mui/material";
import { TaskParam } from "../../types/api";

interface TaskParamFormProps {
  params: TaskParam[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  disabled?: boolean;
}

export const TaskParamForm: React.FC<TaskParamFormProps> = ({
  params,
  values,
  onChange,
  disabled = false,
}) => {
  if (!params || params.length === 0) {
    return null;
  }

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: {
          xs: "1fr",
          sm: params.length === 1 ? "1fr" : params.length === 2 ? "repeat(2, 1fr)" : "repeat(3, 1fr)",
        },
        gap: 2,
        alignItems: "center",
      }}
    >
      {params.map((p) => {
        const val = values[p.key] !== undefined ? values[p.key] : p.default_value || "";

        if (p.type === "select" && p.options && p.options.length > 0) {
          return (
            <FormControl key={p.key} size="small" fullWidth disabled={disabled}>
              <InputLabel id={`param-select-${p.key}`}>{p.label}</InputLabel>
              <Select
                labelId={`param-select-${p.key}`}
                value={val}
                label={p.label}
                onChange={(e) => onChange(p.key, e.target.value)}
                sx={{ backgroundColor: "#FFFFFF", fontSize: "0.85rem" }}
              >
                {p.options.map((opt) => (
                  <MenuItem key={opt} value={opt}>
                    {opt}
                  </MenuItem>
                ))}
              </Select>
              {p.description && (
                <Typography variant="caption" sx={{ color: "#64748B", mt: 0.5, px: 0.5 }}>
                  {p.description}
                </Typography>
              )}
            </FormControl>
          );
        }

        if (p.type === "boolean") {
          return (
            <Box key={p.key} sx={{ display: "flex", flexDirection: "column" }}>
              <FormControlLabel
                control={
                  <Switch
                    checked={val === "true" || val === "1"}
                    onChange={(e) => onChange(p.key, e.target.checked ? "true" : "false")}
                    disabled={disabled}
                    color="primary"
                  />
                }
                label={p.label}
              />
              {p.description && (
                <Typography variant="caption" sx={{ color: "#64748B", px: 0.5 }}>
                  {p.description}
                </Typography>
              )}
            </Box>
          );
        }

        return (
          <TextField
            key={p.key}
            label={p.label}
            size="small"
            fullWidth
            disabled={disabled}
            type={p.type === "number" ? "number" : "text"}
            value={val}
            onChange={(e) => onChange(p.key, e.target.value)}
            placeholder={p.default_value}
            helperText={p.description}
            FormHelperTextProps={{ sx: { fontSize: "0.72rem", color: "#64748B" } }}
            sx={{
              "& .MuiOutlinedInput-root": {
                backgroundColor: "#FFFFFF",
                fontSize: "0.85rem",
              },
            }}
          />
        );
      })}
    </Box>
  );
};
