use std::ffi::OsStr;
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

/// Container runtimes supported by the Apps feature, in order of preference.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ContainerRuntime {
    Docker,
    Podman,
    Nerdctl,
}

impl ContainerRuntime {
    /// CLI binary name used to detect whether the runtime is installed.
    pub fn binary(self) -> &'static str {
        match self {
            ContainerRuntime::Docker => "docker",
            ContainerRuntime::Podman => "podman",
            ContainerRuntime::Nerdctl => "nerdctl",
        }
    }

    /// Full compose command for this runtime. All three runtimes expose a
    /// docker-compatible `compose` subcommand.
    pub fn compose_command(self) -> &'static str {
        match self {
            ContainerRuntime::Docker => "docker compose",
            ContainerRuntime::Podman => "podman compose",
            ContainerRuntime::Nerdctl => "nerdctl compose",
        }
    }
}

/// Detection order is intentional: Docker keeps working exactly as before,
/// and Podman/Nerdctl are used on hosts where Docker is not installed.
const RUNTIME_CANDIDATES: [ContainerRuntime; 3] = [
    ContainerRuntime::Docker,
    ContainerRuntime::Podman,
    ContainerRuntime::Nerdctl,
];

/// Detect the first supported container CLI available in `PATH`.
/// Falls back to Docker (the historical default) when none is found, so the
/// resulting error message still points users at the expected dependency.
pub fn detect_container_runtime() -> ContainerRuntime {
    detect_container_runtime_in(std::env::var_os("PATH").as_deref())
        .unwrap_or(ContainerRuntime::Docker)
}

/// Compose command (e.g. `docker compose`, `podman compose`, `nerdctl compose`)
/// for the first supported container runtime found in `PATH`.
pub fn compose_command() -> &'static str {
    detect_container_runtime().compose_command()
}

fn detect_container_runtime_in(path: Option<&OsStr>) -> Option<ContainerRuntime> {
    let path = path?;
    RUNTIME_CANDIDATES
        .iter()
        .copied()
        .find(|runtime| binary_in_path(runtime.binary(), path))
}

fn binary_in_path(binary: &str, path: &OsStr) -> bool {
    std::env::split_paths(path).any(|dir| is_executable(&dir.join(binary)))
}

fn is_executable(path: &Path) -> bool {
    fs::metadata(path)
        .map(|meta| meta.is_file() && meta.permissions().mode() & 0o111 != 0)
        .unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    fn make_executable(dir: &Path, name: &str) {
        let path = dir.join(name);
        let mut file = fs::File::create(&path).unwrap();
        writeln!(file, "#!/bin/sh").unwrap();
        let mut perms = file.metadata().unwrap().permissions();
        perms.set_mode(0o755);
        fs::set_permissions(&path, perms).unwrap();
    }

    fn path_var(dirs: &[&Path]) -> std::ffi::OsString {
        std::env::join_paths(dirs.iter().map(|dir| dir.as_os_str())).unwrap()
    }

    #[test]
    fn prefers_docker_when_all_runtimes_are_available() {
        let tmp = tempfile::tempdir().unwrap();
        make_executable(tmp.path(), "docker");
        make_executable(tmp.path(), "podman");
        make_executable(tmp.path(), "nerdctl");

        let path = path_var(&[tmp.path()]);
        assert_eq!(
            detect_container_runtime_in(Some(path.as_os_str())),
            Some(ContainerRuntime::Docker)
        );
    }

    #[test]
    fn detects_podman_when_docker_is_missing() {
        let tmp = tempfile::tempdir().unwrap();
        make_executable(tmp.path(), "podman");

        let path = path_var(&[tmp.path()]);
        assert_eq!(
            detect_container_runtime_in(Some(path.as_os_str())),
            Some(ContainerRuntime::Podman)
        );
        assert_eq!(ContainerRuntime::Podman.compose_command(), "podman compose");
    }

    #[test]
    fn detects_nerdctl_when_docker_and_podman_are_missing() {
        let tmp = tempfile::tempdir().unwrap();
        make_executable(tmp.path(), "nerdctl");

        let path = path_var(&[tmp.path()]);
        assert_eq!(
            detect_container_runtime_in(Some(path.as_os_str())),
            Some(ContainerRuntime::Nerdctl)
        );
        assert_eq!(
            ContainerRuntime::Nerdctl.compose_command(),
            "nerdctl compose"
        );
    }

    #[test]
    fn searches_all_path_entries() {
        let docker_dir = tempfile::tempdir().unwrap();
        let podman_dir = tempfile::tempdir().unwrap();
        make_executable(podman_dir.path(), "podman");

        let path = path_var(&[docker_dir.path(), podman_dir.path()]);
        assert_eq!(
            detect_container_runtime_in(Some(path.as_os_str())),
            Some(ContainerRuntime::Podman)
        );
    }

    #[test]
    fn ignores_non_executable_files_and_directories() {
        let tmp = tempfile::tempdir().unwrap();
        fs::write(tmp.path().join("docker"), "not executable").unwrap();
        fs::create_dir(tmp.path().join("podman")).unwrap();

        let path = path_var(&[tmp.path()]);
        assert_eq!(detect_container_runtime_in(Some(path.as_os_str())), None);
    }

    #[test]
    fn detects_default_runtime_when_nothing_is_installed() {
        assert_eq!(compose_command(), "docker compose");
    }
}
