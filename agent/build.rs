fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Without an explicit rerun trigger for the build script itself, cargo can
    // keep using a generated proto that predates schema changes (a new field
    // then silently disappears from the wire format).
    println!("cargo:rerun-if-changed=build.rs");
    println!("cargo:rerun-if-changed=../proto/agent.proto");
    if let Ok(protoc_path) = protoc_bin_vendored::protoc_bin_path() {
        std::env::set_var("PROTOC", protoc_path);
    }
    prost_build::compile_protos(&["../proto/agent.proto"], &["../proto"])?;
    Ok(())
}
