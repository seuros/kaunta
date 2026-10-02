#![allow(non_camel_case_types, non_snake_case)]

use state_machines::state_machine;

state_machine! {
    name: ApplicationLifecycle,
    dynamic: true,
    initial: Booting,
    states: [
        Booting,
        Configured,
        Connected,
        Migrated,
        Serving,
        Draining,
        Stopped,
        ApplicationFailed,
    ],
    events {
        configure {
            transition: { from: Booting, to: Configured }
        }
        connect {
            transition: { from: Configured, to: Connected }
        }
        migrate {
            transition: { from: Connected, to: Migrated }
        }
        start {
            transition: { from: Migrated, to: Serving }
        }
        drain {
            transition: { from: Serving, to: Draining }
        }
        stop {
            transition: { from: Draining, to: Stopped }
        }
        fail {
            transition: {
                from: [Booting, Configured, Connected, Migrated, Serving, Draining],
                to: ApplicationFailed
            }
        }
    }
}

state_machine! {
    name: SetupLifecycle,
    dynamic: true,
    initial: Checking,
    states: [
        Checking,
        NeedsConfiguration,
        NeedsDatabase,
        NeedsUser,
        Complete,
        SetupFailed,
    ],
    events {
        require_configuration {
            transition: { from: Checking, to: NeedsConfiguration }
        }
        require_database {
            transition: { from: [Checking, NeedsConfiguration], to: NeedsDatabase }
        }
        require_user {
            transition: { from: [Checking, NeedsConfiguration, NeedsDatabase], to: NeedsUser }
        }
        complete {
            transition: {
                from: [Checking, NeedsConfiguration, NeedsDatabase, NeedsUser],
                to: Complete
            }
        }
        fail {
            transition: {
                from: [Checking, NeedsConfiguration, NeedsDatabase, NeedsUser],
                to: SetupFailed
            }
        }
    }
}

#[cfg(test)]
mod tests;
