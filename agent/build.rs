fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-changed=../proto/agent.proto");
    if let Ok(protoc_path) = protoc_bin_vendored::protoc_bin_path() {
        std::env::set_var("PROTOC", protoc_path);
    }
    prost_build::compile_protos(&["../proto/agent.proto"], &["../proto"])?;
    Ok(())
}
