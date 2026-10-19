import { type TextFieldProps, IconButton, InputAdornment, TextField } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";

export default function TextFieldWithCopy({
  value,
  copyDisabled,
  ...restProps
}: Omit<TextFieldProps, "slotProps"> & { copyDisabled?: boolean }) {
  return (
    <TextField
      value={value}
      slotProps={{
        input: {
          endAdornment: (
            <InputAdornment position="end">
              <IconButton
                title="Copy"
                disabled={copyDisabled || !value}
                onClick={() => navigator.clipboard.writeText(`${value}`)}
              >
                <ContentCopyIcon fontSize="small" />
              </IconButton>
            </InputAdornment>
          ),
        },
      }}
      {...restProps}
    />
  );
}
