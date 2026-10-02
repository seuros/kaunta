use std::{convert::Infallible, time::Duration};

use rama::{
    Service,
    bytes::Bytes,
    http::{
        Request, Response, StatusCode, header,
        service::web::{extract::State, response::IntoResponse},
        ws::{
            Message,
            handshake::server::{ServerWebSocket, WebSocketAcceptor},
        },
    },
    service::service_fn,
};
use tokio::sync::broadcast;

use crate::web::{
    AppState,
    http::{header_string, origin_trusted},
};

const BROADCAST_CAPACITY: usize = 512;
const RECONNECT_DELAY: Duration = Duration::from_secs(5);
const PING_INTERVAL: Duration = Duration::from_secs(30);

#[derive(Debug, Clone)]
pub struct RealtimeHub {
    sender: broadcast::Sender<String>,
}

impl RealtimeHub {
    #[must_use]
    pub fn new() -> Self {
        let (sender, _) = broadcast::channel(BROADCAST_CAPACITY);
        Self { sender }
    }

    #[must_use]
    pub fn subscribe(&self) -> broadcast::Receiver<String> {
        self.sender.subscribe()
    }

    fn publish(&self, payload: String) {
        if self.sender.send(payload).is_err() {
            tracing::trace!("dropping realtime payload because no websocket clients are connected");
        }
    }
}

impl Default for RealtimeHub {
    fn default() -> Self {
        Self::new()
    }
}

pub async fn listen(pool: sqlx::PgPool, hub: RealtimeHub) {
    loop {
        let mut listener = match crate::db::realtime::connect_listener(&pool).await {
            Ok(listener) => {
                tracing::info!(
                    channel = crate::db::realtime::CHANNEL_NAME,
                    "realtime PostgreSQL listener connected"
                );
                listener
            }
            Err(error) => {
                tracing::warn!(?error, "failed to connect realtime PostgreSQL listener");
                tokio::time::sleep(RECONNECT_DELAY).await;
                continue;
            }
        };

        loop {
            match listener.recv().await {
                Ok(notification) => hub.publish(notification.payload().to_owned()),
                Err(error) => {
                    tracing::warn!(?error, "realtime PostgreSQL listener disconnected");
                    break;
                }
            }
        }

        tokio::time::sleep(RECONNECT_DELAY).await;
    }
}

pub async fn websocket(State(state): State<AppState>, request: Request) -> Response {
    let origin = header_string(request.headers(), header::ORIGIN);
    if !origin_trusted(&state, &origin).await {
        return StatusCode::FORBIDDEN.into_response();
    }

    let hub = state.realtime.clone();
    let service = WebSocketAcceptor::new()
        .with_protocols_flex(true)
        .into_service(service_fn(move |socket: ServerWebSocket| {
            let receiver = hub.subscribe();
            async move {
                serve_socket(socket, receiver).await;
                Ok::<(), Infallible>(())
            }
        }));

    match service.serve(request).await {
        Ok(response) => response,
        Err(never) => match never {},
    }
}

async fn serve_socket(mut socket: ServerWebSocket, mut receiver: broadcast::Receiver<String>) {
    let mut ping = tokio::time::interval(PING_INTERVAL);
    ping.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
    ping.tick().await;

    loop {
        tokio::select! {
            incoming = socket.recv_message() => {
                match incoming {
                    Ok(Message::Close(_)) => return,
                    Ok(Message::Ping(payload)) => {
                        if socket.send_message(Message::Pong(payload)).await.is_err() {
                            return;
                        }
                    }
                    Ok(Message::Text(_) | Message::Binary(_) | Message::Pong(_) | Message::Frame(_)) => {}
                    Err(error) => {
                        tracing::debug!(?error, "realtime websocket disconnected");
                        return;
                    }
                }
            }
            payload = receiver.recv() => {
                match payload {
                    Ok(payload) => {
                        if socket.send_message(Message::text(payload)).await.is_err() {
                            return;
                        }
                    }
                    Err(broadcast::error::RecvError::Lagged(skipped)) => {
                        tracing::warn!(skipped, "realtime websocket client lagged behind");
                    }
                    Err(broadcast::error::RecvError::Closed) => return,
                }
            }
            _ = ping.tick() => {
                if socket.send_message(Message::Ping(Bytes::new())).await.is_err() {
                    return;
                }
            }
        }
    }
}

#[cfg(test)]
mod tests;
