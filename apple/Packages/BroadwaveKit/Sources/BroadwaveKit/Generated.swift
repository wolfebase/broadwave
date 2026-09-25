// Generated from api/openapi.yaml. Do not edit.

import Foundation

public struct Airing: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var channelId: Int64
    public var title: String
    public var subtitle: String?
    public var description: String?
    public var category: String?
    public var programId: String?
    public var new: Bool?
    public var imageUrl: String?
    public var imageWidth: Int?
    public var imageHeight: Int?
    public var season: Int?
    public var episode: Int?
    public var episodeLabel: String?
    public var originalAir: String?
    public var seriesId: String?
    public var live: Bool?
    public var premiere: Bool?
    public var finale: Bool?
    public var rating: String?
    public var cast: String?
    public var gameId: String?
    public var guideSource: String?
    public var guideNumber: String?
    public var channelName: String?
    public var start: Date
    public var end: Date

    public init(id: Int64, channelId: Int64, title: String, subtitle: String? = nil, description: String? = nil, category: String? = nil, programId: String? = nil, new: Bool? = nil, imageUrl: String? = nil, imageWidth: Int? = nil, imageHeight: Int? = nil, season: Int? = nil, episode: Int? = nil, episodeLabel: String? = nil, originalAir: String? = nil, seriesId: String? = nil, live: Bool? = nil, premiere: Bool? = nil, finale: Bool? = nil, rating: String? = nil, cast: String? = nil, gameId: String? = nil, guideSource: String? = nil, guideNumber: String? = nil, channelName: String? = nil, start: Date, end: Date) {
        self.id = id
        self.channelId = channelId
        self.title = title
        self.subtitle = subtitle
        self.description = description
        self.category = category
        self.programId = programId
        self.new = new
        self.imageUrl = imageUrl
        self.imageWidth = imageWidth
        self.imageHeight = imageHeight
        self.season = season
        self.episode = episode
        self.episodeLabel = episodeLabel
        self.originalAir = originalAir
        self.seriesId = seriesId
        self.live = live
        self.premiere = premiere
        self.finale = finale
        self.rating = rating
        self.cast = cast
        self.gameId = gameId
        self.guideSource = guideSource
        self.guideNumber = guideNumber
        self.channelName = channelName
        self.start = start
        self.end = end
    }
}

public struct Caps: Codable, Sendable, Hashable {
    public var platform: String
    public var video: [String]
    public var audio: [String]
    public var maxHeight: Int?
    public var network: String?

    public init(platform: String, video: [String], audio: [String], maxHeight: Int? = nil, network: String? = nil) {
        self.platform = platform
        self.video = video
        self.audio = audio
        self.maxHeight = maxHeight
        self.network = network
    }
}

public struct CatalogBackup: Codable, Sendable, Hashable {
    public var name: String
    public var kind: String
    public var takenAt: Date
    public var bytes: Int64

    public init(name: String, kind: String, takenAt: Date, bytes: Int64) {
        self.name = name
        self.kind = kind
        self.takenAt = takenAt
        self.bytes = bytes
    }
}

public struct Channel: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var deviceId: String
    public var guideNumber: String
    public var guideName: String
    public var displayNumber: String
    public var displayName: String
    public var videoCodec: String?
    public var audioCodec: String?
    public var hd: Bool
    public var favorite: Bool
    public var enabled: Bool
    public var hidden: Bool
    public var present: Bool
    public var guideKey: String?
    public var artUrl: String?
    public var artWidth: Int?
    public var artHeight: Int?
    public var network: String?

    public init(id: Int64, deviceId: String, guideNumber: String, guideName: String, displayNumber: String, displayName: String, videoCodec: String? = nil, audioCodec: String? = nil, hd: Bool, favorite: Bool, enabled: Bool, hidden: Bool, present: Bool, guideKey: String? = nil, artUrl: String? = nil, artWidth: Int? = nil, artHeight: Int? = nil, network: String? = nil) {
        self.id = id
        self.deviceId = deviceId
        self.guideNumber = guideNumber
        self.guideName = guideName
        self.displayNumber = displayNumber
        self.displayName = displayName
        self.videoCodec = videoCodec
        self.audioCodec = audioCodec
        self.hd = hd
        self.favorite = favorite
        self.enabled = enabled
        self.hidden = hidden
        self.present = present
        self.guideKey = guideKey
        self.artUrl = artUrl
        self.artWidth = artWidth
        self.artHeight = artHeight
        self.network = network
    }
}

public struct ChannelPatch: Codable, Sendable, Hashable {
    public var favorite: Bool?
    public var enabled: Bool?
    public var hidden: Bool?
    public var customName: String?
    public var customNumber: String?
    public var guideKey: String?

    public init(favorite: Bool? = nil, enabled: Bool? = nil, hidden: Bool? = nil, customName: String? = nil, customNumber: String? = nil, guideKey: String? = nil) {
        self.favorite = favorite
        self.enabled = enabled
        self.hidden = hidden
        self.customName = customName
        self.customNumber = customNumber
        self.guideKey = guideKey
    }
}

public struct ChannelSignal: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var number: String
    public var name: String
    public var frequencyHz: Int?
    public var strength: Int?
    public var quality: Int?
    public var symbol: Int?
    public var verdict: String?
    public var tip: String?
    public var live: Bool?
    public var checkedAt: Date?

    public init(channelId: Int64, number: String, name: String, frequencyHz: Int? = nil, strength: Int? = nil, quality: Int? = nil, symbol: Int? = nil, verdict: String? = nil, tip: String? = nil, live: Bool? = nil, checkedAt: Date? = nil) {
        self.channelId = channelId
        self.number = number
        self.name = name
        self.frequencyHz = frequencyHz
        self.strength = strength
        self.quality = quality
        self.symbol = symbol
        self.verdict = verdict
        self.tip = tip
        self.live = live
        self.checkedAt = checkedAt
    }
}

public struct Device: Codable, Sendable, Hashable {
    public var deviceId: String
    public var friendlyName: String
    public var modelNumber: String?
    public var firmwareName: String?
    public var firmwareVersion: String?
    public var upgradeAvailable: String?
    public var baseUrl: String
    public var lineupUrl: String?
    public var tunerCount: Int
    public var priority: Int?
    public var lastSeen: String?
    public var note: String?

    public init(deviceId: String, friendlyName: String, modelNumber: String? = nil, firmwareName: String? = nil, firmwareVersion: String? = nil, upgradeAvailable: String? = nil, baseUrl: String, lineupUrl: String? = nil, tunerCount: Int, priority: Int? = nil, lastSeen: String? = nil, note: String? = nil) {
        self.deviceId = deviceId
        self.friendlyName = friendlyName
        self.modelNumber = modelNumber
        self.firmwareName = firmwareName
        self.firmwareVersion = firmwareVersion
        self.upgradeAvailable = upgradeAvailable
        self.baseUrl = baseUrl
        self.lineupUrl = lineupUrl
        self.tunerCount = tunerCount
        self.priority = priority
        self.lastSeen = lastSeen
        self.note = note
    }
}

public struct DeviceHealth: Codable, Sendable, Hashable {
    public var deviceId: String
    public var model: String
    public var firmwareVersion: String
    public var tuners: [TunerLock]
    public var error: String?

    public init(deviceId: String, model: String, firmwareVersion: String, tuners: [TunerLock], error: String? = nil) {
        self.deviceId = deviceId
        self.model = model
        self.firmwareVersion = firmwareVersion
        self.tuners = tuners
        self.error = error
    }
}

public struct Event: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var at: Date
    public var kind: String
    public var message: String

    public init(id: Int64, at: Date, kind: String, message: String) {
        self.id = id
        self.at = at
        self.kind = kind
        self.message = message
    }
}

public struct Game: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var league: String
    public var name: String
    public var shortName: String?
    public var start: Date
    public var state: String
    public var completed: Bool?
    public var detail: String?
    public var clock: String?
    public var period: Int?
    public var broadcasts: [String]?
    public var teams: [SportsTeam]?
    public var redZone: Bool?
    public var powerPlay: Bool?
    public var situation: String?

    public init(id: String, league: String, name: String, shortName: String? = nil, start: Date, state: String, completed: Bool? = nil, detail: String? = nil, clock: String? = nil, period: Int? = nil, broadcasts: [String]? = nil, teams: [SportsTeam]? = nil, redZone: Bool? = nil, powerPlay: Bool? = nil, situation: String? = nil) {
        self.id = id
        self.league = league
        self.name = name
        self.shortName = shortName
        self.start = start
        self.state = state
        self.completed = completed
        self.detail = detail
        self.clock = clock
        self.period = period
        self.broadcasts = broadcasts
        self.teams = teams
        self.redZone = redZone
        self.powerPlay = powerPlay
        self.situation = situation
    }
}

public struct Marker: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var recordingId: Int64?
    public var start: Double
    public var end: Double

    public init(id: Int64, recordingId: Int64? = nil, start: Double, end: Double) {
        self.id = id
        self.recordingId = recordingId
        self.start = start
        self.end = end
    }
}

public struct MultiviewPlanBlocked: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var holders: [String]
    public var reason: String

    public init(channelId: Int64, holders: [String], reason: String) {
        self.channelId = channelId
        self.holders = holders
        self.reason = reason
    }
}

public struct MultiviewPlanOffers: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var cost: String
    public var label: String

    public init(channelId: Int64, cost: String, label: String) {
        self.channelId = channelId
        self.cost = cost
        self.label = label
    }
}

public struct MultiviewPlanPlayable: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var frequencyHz: Int
    public var shared: Bool

    public init(channelId: Int64, frequencyHz: Int, shared: Bool) {
        self.channelId = channelId
        self.frequencyHz = frequencyHz
        self.shared = shared
    }
}

public struct MultiviewPlanStops: Codable, Sendable, Hashable {
    public var at: Date
    public var channelId: Int64
    public var reason: String

    public init(at: Date, channelId: Int64, reason: String) {
        self.at = at
        self.channelId = channelId
        self.reason = reason
    }
}

public struct MultiviewPlan: Codable, Sendable, Hashable {
    public var playable: [MultiviewPlanPlayable]
    public var blocked: [MultiviewPlanBlocked]
    public var tunersNeeded: Int
    public var tunersFree: Int
    public var note: String?
    public var offers: [MultiviewPlanOffers]?
    public var stops: [MultiviewPlanStops]?

    public init(playable: [MultiviewPlanPlayable], blocked: [MultiviewPlanBlocked], tunersNeeded: Int, tunersFree: Int, note: String? = nil, offers: [MultiviewPlanOffers]? = nil, stops: [MultiviewPlanStops]? = nil) {
        self.playable = playable
        self.blocked = blocked
        self.tunersNeeded = tunersNeeded
        self.tunersFree = tunersFree
        self.note = note
        self.offers = offers
        self.stops = stops
    }
}

public struct Pass: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var title: String
    public var channelId: Int64?
    public var kind: String?
    public var padBefore: Int?
    public var padAfter: Int?
    public var priority: Int?
    public var episodes: String?
    public var keepMode: String?
    public var keepCount: Int?
    public var limitCount: Int?
    public var rerecord: Bool?
    public var commercials: Bool?
    public var timeStart: String?
    public var timeEnd: String?
    public var matchKind: String?

    public init(id: Int64, title: String, channelId: Int64? = nil, kind: String? = nil, padBefore: Int? = nil, padAfter: Int? = nil, priority: Int? = nil, episodes: String? = nil, keepMode: String? = nil, keepCount: Int? = nil, limitCount: Int? = nil, rerecord: Bool? = nil, commercials: Bool? = nil, timeStart: String? = nil, timeEnd: String? = nil, matchKind: String? = nil) {
        self.id = id
        self.title = title
        self.channelId = channelId
        self.kind = kind
        self.padBefore = padBefore
        self.padAfter = padAfter
        self.priority = priority
        self.episodes = episodes
        self.keepMode = keepMode
        self.keepCount = keepCount
        self.limitCount = limitCount
        self.rerecord = rerecord
        self.commercials = commercials
        self.timeStart = timeStart
        self.timeEnd = timeEnd
        self.matchKind = matchKind
    }
}

public struct PassList: Codable, Sendable, Hashable {
    public var passes: [Pass]

    public init(passes: [Pass]) {
        self.passes = passes
    }
}

public typealias PictureMode = String

public struct PlannedAiring: Codable, Sendable, Hashable {
    public var passId: Int64
    public var airing: Airing
    public var priority: Int
    public var padBefore: Int
    public var padAfter: Int
    public var conflict: Bool
    public var skipped: Bool
    public var reason: String?
    public var suggestion: Suggestion?

    public init(passId: Int64, airing: Airing, priority: Int, padBefore: Int, padAfter: Int, conflict: Bool, skipped: Bool, reason: String? = nil, suggestion: Suggestion? = nil) {
        self.passId = passId
        self.airing = airing
        self.priority = priority
        self.padBefore = padBefore
        self.padAfter = padAfter
        self.conflict = conflict
        self.skipped = skipped
        self.reason = reason
        self.suggestion = suggestion
    }
}

public struct Recording: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var channelId: Int64
    public var guideNumber: String
    public var title: String
    public var subtitle: String?
    public var description: String?
    public var category: String?
    public var programId: String?
    public var gameId: String?
    public var status: String
    public var error: String?
    public var startedAt: Date
    public var endsAt: Date?
    public var endedAt: Date?
    public var bytes: Int64?
    public var position: Double?
    public var durationSec: Double?
    public var watched: Int?

    public init(id: Int64, channelId: Int64, guideNumber: String, title: String, subtitle: String? = nil, description: String? = nil, category: String? = nil, programId: String? = nil, gameId: String? = nil, status: String, error: String? = nil, startedAt: Date, endsAt: Date? = nil, endedAt: Date? = nil, bytes: Int64? = nil, position: Double? = nil, durationSec: Double? = nil, watched: Int? = nil) {
        self.id = id
        self.channelId = channelId
        self.guideNumber = guideNumber
        self.title = title
        self.subtitle = subtitle
        self.description = description
        self.category = category
        self.programId = programId
        self.gameId = gameId
        self.status = status
        self.error = error
        self.startedAt = startedAt
        self.endsAt = endsAt
        self.endedAt = endedAt
        self.bytes = bytes
        self.position = position
        self.durationSec = durationSec
        self.watched = watched
    }
}

public struct RoomState: Codable, Sendable, Hashable {
    public var room: String
    public var channelId: Int64?
    public var mode: String
    public var anchorServer: Double
    public var anchorMedia: Double
    public var rate: Double
    public var latency: String
    public var version: Int
    public var members: Int

    public init(room: String, channelId: Int64? = nil, mode: String, anchorServer: Double, anchorMedia: Double, rate: Double, latency: String, version: Int, members: Int) {
        self.room = room
        self.channelId = channelId
        self.mode = mode
        self.anchorServer = anchorServer
        self.anchorMedia = anchorMedia
        self.rate = rate
        self.latency = latency
        self.version = version
        self.members = members
    }
}

public struct ServerInfo: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var version: String
    public var apiVersion: Int
    public var encoder: String?
    public var tunerCount: Int?
    public var features: [String]
    public var minAppVersion: String?
    public var update: ServerUpdate?

    public init(id: String, name: String, version: String, apiVersion: Int, encoder: String? = nil, tunerCount: Int? = nil, features: [String], minAppVersion: String? = nil, update: ServerUpdate? = nil) {
        self.id = id
        self.name = name
        self.version = version
        self.apiVersion = apiVersion
        self.encoder = encoder
        self.tunerCount = tunerCount
        self.features = features
        self.minAppVersion = minAppVersion
        self.update = update
    }
}

public struct ServerUpdate: Codable, Sendable, Hashable {
    public var version: String
    public var notesUrl: String
    public var message: String

    public init(version: String, notesUrl: String, message: String) {
        self.version = version
        self.notesUrl = notesUrl
        self.message = message
    }
}

public struct Settings: Codable, Sendable, Hashable {
    public var layout: String?
    public var recordingsPath: String?
    public var profile: String?
    public var audio: String?
    public var encoder: String?
    public var watermarkGB: String?
    public var pictureMode: PictureMode?
    public var autoplay: String?
    public var hdhrEmulate: String?
    public var setupComplete: String?
    public var needsSetup: String?
    public var lastGuidePull: Date?
    public var nextGuidePull: Date?
    public var lastManualGuidePull: Date?
    public var hideScores: String?
    public var liveScores: String?
    public var checkUpdates: String?
    public var sdUser: String?
    public var sdPassword: String?
    public var sdLineup: String?
    public var sdPasswordSet: String?
    public var guideUrl: String?
    public var tmdbKey: String?
    public var tmdbKeySet: String?
    public var sportsdbKey: String?
    public var sportsdbKeySet: String?

    public init(layout: String? = nil, recordingsPath: String? = nil, profile: String? = nil, audio: String? = nil, encoder: String? = nil, watermarkGB: String? = nil, pictureMode: PictureMode? = nil, autoplay: String? = nil, hdhrEmulate: String? = nil, setupComplete: String? = nil, needsSetup: String? = nil, lastGuidePull: Date? = nil, nextGuidePull: Date? = nil, lastManualGuidePull: Date? = nil, hideScores: String? = nil, liveScores: String? = nil, checkUpdates: String? = nil, sdUser: String? = nil, sdPassword: String? = nil, sdLineup: String? = nil, sdPasswordSet: String? = nil, guideUrl: String? = nil, tmdbKey: String? = nil, tmdbKeySet: String? = nil, sportsdbKey: String? = nil, sportsdbKeySet: String? = nil) {
        self.layout = layout
        self.recordingsPath = recordingsPath
        self.profile = profile
        self.audio = audio
        self.encoder = encoder
        self.watermarkGB = watermarkGB
        self.pictureMode = pictureMode
        self.autoplay = autoplay
        self.hdhrEmulate = hdhrEmulate
        self.setupComplete = setupComplete
        self.needsSetup = needsSetup
        self.lastGuidePull = lastGuidePull
        self.nextGuidePull = nextGuidePull
        self.lastManualGuidePull = lastManualGuidePull
        self.hideScores = hideScores
        self.liveScores = liveScores
        self.checkUpdates = checkUpdates
        self.sdUser = sdUser
        self.sdPassword = sdPassword
        self.sdLineup = sdLineup
        self.sdPasswordSet = sdPasswordSet
        self.guideUrl = guideUrl
        self.tmdbKey = tmdbKey
        self.tmdbKeySet = tmdbKeySet
        self.sportsdbKey = sportsdbKey
        self.sportsdbKeySet = sportsdbKeySet
    }
}

public struct SetupFinish: Codable, Sendable, Hashable {
    public var running: Bool
    public var ready: String?
    public var channelId: Int64?
    public var steps: [SetupStep]

    public init(running: Bool, ready: String? = nil, channelId: Int64? = nil, steps: [SetupStep]) {
        self.running = running
        self.ready = ready
        self.channelId = channelId
        self.steps = steps
    }
}

public struct SetupStep: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var title: String
    public var state: String
    public var detail: String?

    public init(id: String, title: String, state: String, detail: String? = nil) {
        self.id = id
        self.title = title
        self.state = state
        self.detail = detail
    }
}

public struct Slot: Codable, Sendable, Hashable {
    public var recordingId: Int64
    public var title: String
    public var start: Date
    public var end: Date

    public init(recordingId: Int64, title: String, start: Date, end: Date) {
        self.recordingId = recordingId
        self.title = title
        self.start = start
        self.end = end
    }
}

public struct Source: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var kind: String
    public var name: String
    public var url: String?
    public var xmltvUrl: String?
    public var enabled: Bool
    public var stableKey: String?
    public var priority: Int?
    public var tunerCount: Int?
    public var streamLimit: Int?
    public var streamFormat: String?
    public var hasGuide: Bool?
    public var needsTuner: Bool?
    public var refresh: String?
    public var lastRefresh: String?
    public var health: String?
    public var streamsInUse: Int?
    public var deviceId: String?

    public init(id: Int64, kind: String, name: String, url: String? = nil, xmltvUrl: String? = nil, enabled: Bool, stableKey: String? = nil, priority: Int? = nil, tunerCount: Int? = nil, streamLimit: Int? = nil, streamFormat: String? = nil, hasGuide: Bool? = nil, needsTuner: Bool? = nil, refresh: String? = nil, lastRefresh: String? = nil, health: String? = nil, streamsInUse: Int? = nil, deviceId: String? = nil) {
        self.id = id
        self.kind = kind
        self.name = name
        self.url = url
        self.xmltvUrl = xmltvUrl
        self.enabled = enabled
        self.stableKey = stableKey
        self.priority = priority
        self.tunerCount = tunerCount
        self.streamLimit = streamLimit
        self.streamFormat = streamFormat
        self.hasGuide = hasGuide
        self.needsTuner = needsTuner
        self.refresh = refresh
        self.lastRefresh = lastRefresh
        self.health = health
        self.streamsInUse = streamsInUse
        self.deviceId = deviceId
    }
}

public struct SportsTeam: Codable, Sendable, Hashable {
    public var name: String
    public var short: String?
    public var abbr: String?
    public var score: String?
    public var home: Bool?
    public var color: String?
    public var altColor: String?
    public var logo: String?

    public init(name: String, short: String? = nil, abbr: String? = nil, score: String? = nil, home: Bool? = nil, color: String? = nil, altColor: String? = nil, logo: String? = nil) {
        self.name = name
        self.short = short
        self.abbr = abbr
        self.score = score
        self.home = home
        self.color = color
        self.altColor = altColor
        self.logo = logo
    }
}

public struct StreamInfo: Codable, Sendable, Hashable {
    public var rendition: String
    public var video: String
    public var audio: String
    public var mode: PictureMode?
    public var reason: String
    public var sourceVideo: String?
    public var sourceAudio: String?
    public var encoder: String?
    public var scan: String?
    public var sourceWidth: Int?
    public var sourceHeight: Int?
    public var sourceFps: String?
    public var outputWidth: Int?
    public var outputHeight: Int?
    public var outputFps: String?
    public var bitrate: String?
    public var decode: String?

    public init(rendition: String, video: String, audio: String, mode: PictureMode? = nil, reason: String, sourceVideo: String? = nil, sourceAudio: String? = nil, encoder: String? = nil, scan: String? = nil, sourceWidth: Int? = nil, sourceHeight: Int? = nil, sourceFps: String? = nil, outputWidth: Int? = nil, outputHeight: Int? = nil, outputFps: String? = nil, bitrate: String? = nil, decode: String? = nil) {
        self.rendition = rendition
        self.video = video
        self.audio = audio
        self.mode = mode
        self.reason = reason
        self.sourceVideo = sourceVideo
        self.sourceAudio = sourceAudio
        self.encoder = encoder
        self.scan = scan
        self.sourceWidth = sourceWidth
        self.sourceHeight = sourceHeight
        self.sourceFps = sourceFps
        self.outputWidth = outputWidth
        self.outputHeight = outputHeight
        self.outputFps = outputFps
        self.bitrate = bitrate
        self.decode = decode
    }
}

public struct Suggestion: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var guideNumber: String?
    public var title: String
    public var start: Date
    public var end: Date

    public init(channelId: Int64, guideNumber: String? = nil, title: String, start: Date, end: Date) {
        self.channelId = channelId
        self.guideNumber = guideNumber
        self.title = title
        self.start = start
        self.end = end
    }
}

public struct TeamFollow: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64?
    public var name: String
    public var short: String?
    public var abbr: String?
    public var league: String?
    public var logo: String?
    public var color: String?
    public var record: Bool?

    public init(id: Int64? = nil, name: String, short: String? = nil, abbr: String? = nil, league: String? = nil, logo: String? = nil, color: String? = nil, record: Bool? = nil) {
        self.id = id
        self.name = name
        self.short = short
        self.abbr = abbr
        self.league = league
        self.logo = logo
        self.color = color
        self.record = record
    }
}

public struct TeamList: Codable, Sendable, Hashable {
    public var teams: [TeamFollow]

    public init(teams: [TeamFollow]) {
        self.teams = teams
    }
}

public struct Tuner: Codable, Sendable, Hashable {
    public var index: Int
    public var guide: String?
    public var name: String?
    public var target: String?
    public var ours: Bool
    public var strength: Int?
    public var quality: Int?
    public var symbol: Int?
    public var viewers: Int?

    public init(index: Int, guide: String? = nil, name: String? = nil, target: String? = nil, ours: Bool, strength: Int? = nil, quality: Int? = nil, symbol: Int? = nil, viewers: Int? = nil) {
        self.index = index
        self.guide = guide
        self.name = name
        self.target = target
        self.ours = ours
        self.strength = strength
        self.quality = quality
        self.symbol = symbol
        self.viewers = viewers
    }
}

public struct TunerLock: Codable, Sendable, Hashable {
    public var index: Int
    public var locked: Bool

    public init(index: Int, locked: Bool) {
        self.index = index
        self.locked = locked
    }
}

public struct VirtualChannel: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var number: String
    public var name: String
    public var orderMode: String?
    public var ruleTitle: String?
    public var recordings: [Int64]

    public init(id: Int64, number: String, name: String, orderMode: String? = nil, ruleTitle: String? = nil, recordings: [Int64]) {
        self.id = id
        self.number = number
        self.name = name
        self.orderMode = orderMode
        self.ruleTitle = ruleTitle
        self.recordings = recordings
    }
}

public struct WatchSession: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var playlist: String
    public var rendition: String
    public var stream: StreamInfo
    public var profile: String?
    public var audio: String?
    public var encoder: String
    public var picture: PictureMode?
    public var videoMode: String?
    public var shared: Bool
    public var viewers: Int
    public var frequencyHz: Int?
    public var program: Int?
    public var hints: [String]?
    public var tuners: [Tuner]?

    public init(channelId: Int64, playlist: String, rendition: String, stream: StreamInfo, profile: String? = nil, audio: String? = nil, encoder: String, picture: PictureMode? = nil, videoMode: String? = nil, shared: Bool, viewers: Int, frequencyHz: Int? = nil, program: Int? = nil, hints: [String]? = nil, tuners: [Tuner]? = nil) {
        self.channelId = channelId
        self.playlist = playlist
        self.rendition = rendition
        self.stream = stream
        self.profile = profile
        self.audio = audio
        self.encoder = encoder
        self.picture = picture
        self.videoMode = videoMode
        self.shared = shared
        self.viewers = viewers
        self.frequencyHz = frequencyHz
        self.program = program
        self.hints = hints
        self.tuners = tuners
    }
}
