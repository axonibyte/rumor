//! Test 3's fixture root. It exists to carry one GPL dependency into a graph
//! `cargo deny` will resolve; nothing here is built by the real workspace.
pub fn uses_it() -> &'static str {
    gpl_fixture::name()
}
