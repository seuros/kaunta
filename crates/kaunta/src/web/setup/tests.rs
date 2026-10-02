use super::{SetupForm, build_database_url, validate_setup_form};

fn valid_form() -> SetupForm {
    SetupForm {
        db_host: "localhost".to_owned(),
        db_port: "5432".to_owned(),
        db_name: "kaunta".to_owned(),
        db_user: "postgres".to_owned(),
        db_ssl_mode: "disable".to_owned(),
        server_port: "3000".to_owned(),
        data_dir: "./data".to_owned(),
        admin_username: "admin".to_owned(),
        admin_password: "password123".to_owned(),
        admin_password_confirm: "password123".to_owned(),
        ..SetupForm::default()
    }
}

#[test]
fn validates_and_applies_setup_defaults() {
    let mut form = valid_form();
    form.db_port.clear();
    form.server_port.clear();
    form.data_dir.clear();
    validate_setup_form(&mut form).unwrap();
    assert_eq!(form.db_port, "5432");
    assert_eq!(form.server_port, "3000");
    assert_eq!(form.data_dir, "./data");
}

#[test]
fn rejects_invalid_setup_credentials() {
    let mut form = valid_form();
    form.admin_username = "admin@user".to_owned();
    assert_eq!(
        validate_setup_form(&mut form).unwrap_err(),
        "username can only contain letters, numbers, and underscores"
    );

    let mut form = valid_form();
    form.admin_password_confirm = "different".to_owned();
    assert_eq!(
        validate_setup_form(&mut form).unwrap_err(),
        "passwords do not match"
    );
}

#[test]
fn builds_percent_encoded_postgresql_url() {
    let mut form = valid_form();
    form.db_password = "secret:@".to_owned();
    assert_eq!(
        build_database_url(&form).unwrap(),
        "postgresql://postgres:secret%3A%40@localhost:5432/kaunta?sslmode=disable"
    );
}
