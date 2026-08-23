import SwiftUI
import UIKit

struct ChatMessageCollectionView: UIViewRepresentable {
    let sessionIdentity: SessionIdentity
    let blocks: [ChatBlock]
    let streamingAssistantID: String?
    let isBusy: Bool
    @Binding var isFollowing: Bool
    let scrollRequestRevision: Int
    let onPermissionAnswer: (String, String?) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(owner: self) }

    func makeUIView(context: Context) -> UICollectionView {
        let layout = UICollectionViewCompositionalLayout { _, _ in
            let item = NSCollectionLayoutItem(layoutSize: NSCollectionLayoutSize(
                widthDimension: .fractionalWidth(1), heightDimension: .estimated(Metrics.estimatedHeight)))
            let group = NSCollectionLayoutGroup.vertical(layoutSize: NSCollectionLayoutSize(
                widthDimension: .fractionalWidth(1), heightDimension: .estimated(Metrics.estimatedHeight)), subitems: [item])
            let section = NSCollectionLayoutSection(group: group)
            section.contentInsets = NSDirectionalEdgeInsets(top: Metrics.sectionTop, leading: 0, bottom: Metrics.sectionBottom, trailing: 0)
            return section
        }
        let collection = UICollectionView(frame: .zero, collectionViewLayout: layout)
        collection.backgroundColor = .clear
        collection.alpha = 0
        collection.keyboardDismissMode = .interactive
        collection.alwaysBounceVertical = true
        collection.delegate = context.coordinator
        context.coordinator.install(on: collection)
        return collection
    }

    func updateUIView(_ collection: UICollectionView, context: Context) {
        context.coordinator.update(owner: self, collection: collection)
    }

    static func dismantleUIView(_ collection: UICollectionView, coordinator: Coordinator) { coordinator.stop() }

    final class CellModel: ObservableObject {
        @Published var block: ChatBlock
        @Published var isStreaming: Bool
        let permission: (String, String?) -> Void
        init(block: ChatBlock, isStreaming: Bool, permission: @escaping (String, String?) -> Void) {
            self.block = block
            self.isStreaming = isStreaming
            self.permission = permission
        }
        func update(block: ChatBlock, isStreaming: Bool) -> Bool {
            let changed = self.block != block || self.isStreaming != isStreaming
            if self.block != block { self.block = block }
            if self.isStreaming != isStreaming { self.isStreaming = isStreaming }
            return changed
        }
    }

    @MainActor final class Coordinator: NSObject, UICollectionViewDelegate {
        private var owner: ChatMessageCollectionView
        private weak var collection: UICollectionView?
        private var dataSource: UICollectionViewDiffableDataSource<Int, String>!
        private var models: [String: CellModel] = [:]
        private var displayedIDs: [String] = []
        private var pendingLayout = false
        private var needsFinalFlush = false
        private var displayLink: CADisplayLink?
        private var didAppear = false
        private var lastBusy = false
        private var lastRevision = 0
        private var identity: SessionIdentity
        private var sessionGeneration = 0
        private var renderedBlockIDs = Set<String>()
        private var initialRevealPending = false
        private var initialRevealStartedAt: CFTimeInterval?
        private var initialRevealStableSince: CFTimeInterval?
        private var sessionLoadClampDeadline: CFTimeInterval?
        private var contentObservation: NSKeyValueObservation?
        private lazy var cellRegistration = UICollectionView.CellRegistration<UICollectionViewCell, String> { [weak self] cell, _, identifier in
            guard let self, let model = self.models[identifier] else { return }
            let cellGeneration = self.sessionGeneration
            let cellIdentity = self.identity
            cell.contentConfiguration = UIHostingConfiguration {
                ModelRoot(model: model, onRender: { [weak self] blockID in
                    self?.markdownDidRender(blockID: blockID, generation: cellGeneration, identity: cellIdentity)
                })
            }.margins(.all, 0)
        }

        init(owner: ChatMessageCollectionView) {
            self.owner = owner
            identity = owner.sessionIdentity
            super.init()
        }

        func install(on collection: UICollectionView) {
            self.collection = collection
            let reusableRegistration = cellRegistration
            dataSource = UICollectionViewDiffableDataSource<Int, String>(collectionView: collection) { view, index, identifier in
                view.dequeueConfiguredReusableCell(using: reusableRegistration, for: index, item: identifier)
            }
            contentObservation = collection.observe(\.contentSize, options: [.new, .old]) { [weak self] _, change in
                guard let self, change.newValue != change.oldValue else { return }
                DispatchQueue.main.async { [weak self] in self?.contentSizeChanged() }
            }
        }

        private func contentSizeChanged() {
            guard (didAppear || initialRevealPending), owner.isFollowing else { return }
            if initialRevealPending && owner.streamingAssistantID == nil {
                initialRevealStableSince = nil
                sessionLoadClampDeadline = nil
            }
            requestLayout()
        }

        private func markdownDidRender(blockID: String, generation: Int, identity: SessionIdentity) {
            guard generation == sessionGeneration, identity == self.identity else { return }
            renderedBlockIDs.insert(blockID)
            requestLayout()
        }

        func update(owner: ChatMessageCollectionView, collection: UICollectionView) {
            let identityChanged = identity != owner.sessionIdentity
            self.owner = owner
            if identityChanged {
                identity = owner.sessionIdentity
                sessionGeneration += 1
                didAppear = false
                initialRevealPending = false
                initialRevealStartedAt = nil
                initialRevealStableSince = nil
                sessionLoadClampDeadline = nil
                renderedBlockIDs.removeAll()
                displayedIDs = []
                collection.alpha = 0
                models.removeAll()
            }
            var modelChanged = false
            for block in owner.blocks {
                if let model = models[block.id] {
                    modelChanged = model.update(block: block, isStreaming: block.id == owner.streamingAssistantID) || modelChanged
                } else {
                    models[block.id] = CellModel(block: block, isStreaming: block.id == owner.streamingAssistantID, permission: owner.onPermissionAnswer)
                    modelChanged = true
                }
            }
            let retainedBlockIDs = Set(owner.blocks.map(\.id))
            models = models.filter { retainedBlockIDs.contains($0.key) }
            let blockIDs = owner.blocks.map(\.id)
            if blockIDs != displayedIDs {
                displayedIDs = blockIDs
                var snapshot = NSDiffableDataSourceSnapshot<Int, String>()
                snapshot.appendSections([0])
                snapshot.appendItems(blockIDs)
                let snapshotGeneration = sessionGeneration
                let snapshotIdentity = identity
                dataSource.apply(snapshot, animatingDifferences: false) { [weak self] in
                    guard let self, self.sessionGeneration == snapshotGeneration,
                          self.identity == snapshotIdentity, !self.didAppear else { return }
                    self.initialRevealPending = true
                    self.initialRevealStartedAt = nil
                    self.initialRevealStableSince = nil
                    self.requestLayout()
                }
            } else if modelChanged {
                requestLayout()
            } else if !didAppear && !initialRevealPending {
                initialRevealPending = true
                initialRevealStartedAt = nil
                initialRevealStableSince = nil
                requestLayout()
            }
            if owner.scrollRequestRevision != lastRevision {
                lastRevision = owner.scrollRequestRevision
                owner.isFollowing = true
                pinToBottom()
            }
            if lastBusy && !owner.isBusy { needsFinalFlush = true; requestLayout() }
            lastBusy = owner.isBusy
        }

        private func initialRevealReady() -> Bool {
            guard !owner.blocks.isEmpty else { return true }
            guard let finalBlock = owner.blocks.last else { return true }
            if finalBlock.kind != .assistant { return true }
            return renderedBlockIDs.contains(finalBlock.id)
        }
        func requestLayout() {
            guard !pendingLayout else { return }
            pendingLayout = true
            if displayLink == nil {
                displayLink = CADisplayLink(target: self, selector: #selector(flushLayout))
                displayLink?.add(to: .main, forMode: .common)
            }
        }
        @objc private func flushLayout() {
            displayLink?.invalidate()
            displayLink = nil
            pendingLayout = false
            guard let collection else { return }
            let layoutStartOffset = collection.contentOffset
            UIView.performWithoutAnimation {
                collection.layoutIfNeeded()
                if self.initialRevealPending, collection.numberOfItems(inSection: 0) > 0 {
                    let lastIndex = IndexPath(item: collection.numberOfItems(inSection: 0) - 1, section: 0)
                    collection.scrollToItem(at: lastIndex, at: .bottom, animated: false)
                    collection.layoutIfNeeded()
                }
            }
            if !owner.isFollowing || owner.isBusy || owner.streamingAssistantID != nil {
                sessionLoadClampDeadline = nil
            }
            if owner.isFollowing {
                if let clampDeadline = sessionLoadClampDeadline {
                    if CACurrentMediaTime() < clampDeadline {
                        pinToBottom()
                        requestLayout()
                    } else {
                        sessionLoadClampDeadline = nil
                    }
                } else if needsFinalFlush || initialRevealPending {
                    pinToBottom()
                } else {
                    if !animateStreamingFollow(startingOffset: layoutStartOffset) {
                        pinToBottom()
                    }
                }
            }
            if initialRevealPending && initialRevealReady() {
                let currentTime = CACurrentMediaTime()
                if initialRevealStartedAt == nil {
                    initialRevealStartedAt = currentTime
                }
                if initialRevealStableSince == nil {
                    initialRevealStableSince = currentTime
                }
                let stableDuration = currentTime - (initialRevealStableSince ?? currentTime)
                let elapsedDuration = currentTime - (initialRevealStartedAt ?? currentTime)
                guard stableDuration >= Metrics.initialRevealStableDuration ||
                        elapsedDuration >= Metrics.initialRevealMaximumDuration else {
                    requestLayout()
                    return
                }
                initialRevealPending = false
                initialRevealStartedAt = nil
                initialRevealStableSince = nil
                didAppear = true
                collection.alpha = 1
                if !owner.blocks.isEmpty, !owner.isBusy, owner.streamingAssistantID == nil {
                    sessionLoadClampDeadline = CACurrentMediaTime() + Metrics.sessionLoadClampDuration
                }
            } else if initialRevealPending {
                initialRevealStableSince = nil
            }
            if needsFinalFlush {
                needsFinalFlush = false
                let flushGeneration = sessionGeneration
                let flushIdentity = identity
                DispatchQueue.main.async { [weak self] in
                    guard let self, self.sessionGeneration == flushGeneration, self.identity == flushIdentity,
                          let collection = self.collection else { return }
                    UIView.performWithoutAnimation {
                        collection.layoutIfNeeded()
                        if self.owner.isFollowing {
                            self.pinToBottom()
                        }
                    }
                }
            }
        }

        private func animateStreamingFollow(startingOffset: CGPoint) -> Bool {
            guard didAppear, !initialRevealPending, owner.isBusy,
                  owner.streamingAssistantID != nil, owner.isFollowing,
                  let collection, !collection.isTracking, !collection.isDragging,
                  !collection.isDecelerating else { return false }
            let maximumOffset = max(-collection.adjustedContentInset.top,
                                    collection.contentSize.height - collection.bounds.height + collection.adjustedContentInset.bottom)
            let offsetDelta = maximumOffset - startingOffset.y
            if abs(offsetDelta) <= Metrics.streamingOffsetThreshold { return true }
            guard offsetDelta > 0 else { return false }
            UIView.performWithoutAnimation {
                collection.setContentOffset(startingOffset, animated: false)
            }
            UIView.animate(withDuration: Metrics.streamingAnimationDuration,
                           delay: 0,
                           options: [.beginFromCurrentState, .curveEaseOut, .allowUserInteraction]) {
                collection.setContentOffset(CGPoint(x: collection.contentOffset.x, y: maximumOffset), animated: false)
            }
            return true
        }
        private func pinToBottom() {
            guard let collection else { return }
            collection.layer.removeAllAnimations()
            UIView.performWithoutAnimation {
                collection.layoutIfNeeded()
                let offset = max(-collection.adjustedContentInset.top, collection.contentSize.height - collection.bounds.height + collection.adjustedContentInset.bottom)
                collection.setContentOffset(CGPoint(x: 0, y: offset), animated: false)
            }
        }
        func stop() {
            displayLink?.invalidate()
            displayLink = nil
            sessionLoadClampDeadline = nil
            contentObservation?.invalidate()
            contentObservation = nil
        }
        deinit {
            displayLink?.invalidate()
            contentObservation?.invalidate()
        }
        func scrollViewWillBeginDragging(_ scrollView: UIScrollView) { owner.isFollowing = false }
        func scrollViewDidEndDragging(_ scrollView: UIScrollView, willDecelerate decelerate: Bool) { if !decelerate { restoreIfNearBottom(scrollView) } }
        func scrollViewDidEndDecelerating(_ scrollView: UIScrollView) { restoreIfNearBottom(scrollView) }
        private func restoreIfNearBottom(_ scrollView: UIScrollView) {
            let maximumOffset = max(-scrollView.adjustedContentInset.top, scrollView.contentSize.height - scrollView.bounds.height + scrollView.adjustedContentInset.bottom)
            if maximumOffset - scrollView.contentOffset.y <= Metrics.bottomTolerance { owner.isFollowing = true }
        }
    }
}

private struct ModelRoot: View {
    @ObservedObject var model: ChatMessageCollectionView.CellModel
    let onRender: (String) -> Void
    var body: some View {
        ChatBlockView(block: model.block, isStreaming: model.isStreaming,
                      onRender: { onRender(model.block.id) }, onPermissionAnswer: model.permission)
            .id(model.block.id)
    }
}

private enum Metrics {
    static let estimatedHeight: CGFloat = 44
    static let sectionTop: CGFloat = 8
    static let sectionBottom: CGFloat = 12
    static let bottomTolerance: CGFloat = 18
    static let streamingAnimationDuration: TimeInterval = 0.20
    static let streamingOffsetThreshold: CGFloat = 1
    static let initialRevealStableDuration: CFTimeInterval = 0.30
    static let initialRevealMaximumDuration: CFTimeInterval = 2.0
    static let sessionLoadClampDuration: CFTimeInterval = 8.0
}
