import AnyAICLIRemoteCore
import Foundation

struct ConnectionContext {
  let deviceID: UUID
  let generation: UUID
}

struct SessionContext {
  let connection: ConnectionContext
  let sessionIdentity: SessionIdentity
  let generation: UUID
}
