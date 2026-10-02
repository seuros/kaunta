use super::{ApplicationLifecycle, ApplicationLifecycleState, SetupLifecycle, SetupLifecycleState};

#[test]
fn application_happy_path_is_ordered() {
    let lifecycle = ApplicationLifecycle::new(());
    let lifecycle = lifecycle.configure().expect("configure");
    let lifecycle = lifecycle.connect().expect("connect");
    let lifecycle = lifecycle.migrate().expect("migrate");
    let lifecycle = lifecycle.start().expect("start");
    let lifecycle = lifecycle.drain().expect("drain");
    let lifecycle = lifecycle.stop().expect("stop");

    assert_eq!(
        lifecycle.into_dynamic().current_state(),
        ApplicationLifecycleState::Stopped
    );
}

#[test]
fn setup_can_complete_after_user_creation() {
    let setup = SetupLifecycle::new(());
    let setup = setup.require_user().expect("require user");
    let setup = setup.complete().expect("complete");

    assert_eq!(
        setup.into_dynamic().current_state(),
        SetupLifecycleState::Complete
    );
}
