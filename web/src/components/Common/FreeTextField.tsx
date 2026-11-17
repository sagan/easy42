import Autocomplete from "@mui/material/Autocomplete";
import { type TextFieldProps, IconButton, InputAdornment, TextField } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";

// 1. Define the props, extending standard MUI TextField props for maximum flexibility
interface FreeTextFieldProps extends Omit<TextFieldProps, "onChange"> {
  value: string;
  onChange: (value: string) => void;
  options: readonly string[];
}

export default function FreeTextField({ value, onChange, options, label, sx, ...restProps }: FreeTextFieldProps) {
  return (
    <Autocomplete
      freeSolo
      sx={sx}
      options={options}
      value={value}
      // Catches dropdown clicks or 'Enter' selections
      onChange={(_, newValue) => {
        onChange(newValue || "");
      }}
      renderInput={(params) => (
        <TextField
          {...params}
          {...restProps} // Passes down variant, helperText, error, width, etc.
          label={label}
          // Catches raw manual typing
          onChange={(e) => onChange(e.target.value)}
          slotProps={{
            ...params.InputProps,
            input: {
              ...params.InputProps,
              endAdornment: (
                <>
                  {params.InputProps.endAdornment}
                  <InputAdornment position="end">
                    <IconButton
                      title="Copy"
                      disabled={!value}
                      onClick={() => navigator.clipboard.writeText(`${value}`)}
                    >
                      <ContentCopyIcon fontSize="small" />
                    </IconButton>
                  </InputAdornment>
                </>
              ),
            },
          }}
        />
      )}
    />
  );
}
