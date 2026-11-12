#include <QtWidgets>
#include <QImageReader>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <algorithm>
#include <cstdio>

#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif

namespace {

class Banner : public QWidget {
public:
    Banner(QString name, const QString &file) : title(std::move(name)) {
        QImageReader reader(file);
        const auto size = reader.size();
        if (size.width() > 1520)
            reader.setScaledSize(size.scaled(1520, 1024, Qt::KeepAspectRatio));
        artwork = reader.read();
        setFixedHeight(112);
    }

protected:
    void paintEvent(QPaintEvent *) override {
        QPainter painter(this);
        painter.setRenderHint(QPainter::SmoothPixmapTransform);
        painter.fillRect(rect(), QColor("#152c3d"));
        if (!artwork.isNull()) {
            const auto scaled = artwork.size().scaled(size(), Qt::KeepAspectRatioByExpanding);
            painter.drawImage(QRect(QPoint((width() - scaled.width()) / 2, (height() - scaled.height()) / 2), scaled), artwork);
        }
        QLinearGradient fade(0, 0, width() * 0.75, 0);
        fade.setColorAt(0, QColor(13, 24, 37, 245));
        fade.setColorAt(1, QColor(13, 24, 37, 12));
        painter.fillRect(rect(), fade);
        auto titleFont = font();
        titleFont.setPointSize(22);
        titleFont.setBold(true);
        painter.setFont(titleFont);
        painter.setPen(Qt::white);
        painter.drawText(rect().adjusted(24, 0, -24, 0), Qt::AlignVCenter, title);
    }

private:
    QString title;
    QImage artwork;
};

class DisclosureButton : public QToolButton {
public:
    DisclosureButton() {
        setCheckable(true);
        setToolButtonStyle(Qt::ToolButtonTextBesideIcon);
        setIconSize(QSize(16, 10));
        connect(this, &QToolButton::toggled, this, [this] { updateArrow(); });
        updateArrow();
    }

protected:
    void changeEvent(QEvent *event) override {
        QToolButton::changeEvent(event);
        if (event->type() == QEvent::PaletteChange || event->type() == QEvent::StyleChange)
            updateArrow();
    }

private:
    void updateArrow() {
        // Rasterize the theme's arrow before scaling it. Some styles use a
        // fixed-size glyph even when given a smaller drawing rectangle.
        QStyleOptionToolButton option;
        initStyleOption(&option);
        option.rect = QRect(0, 0, 18, 18);
        constexpr qreal scale = 2;
        QPixmap arrow(option.rect.size() * scale);
        arrow.setDevicePixelRatio(scale);
        arrow.fill(Qt::transparent);
        {
            QPainter painter(&arrow);
            const auto primitive = isChecked() ? QStyle::PE_IndicatorArrowDown : QStyle::PE_IndicatorArrowRight;
            style()->drawPrimitive(primitive, &option, &painter, this);
        }

        QPixmap icon(iconSize() * scale);
        icon.setDevicePixelRatio(scale);
        icon.fill(Qt::transparent);
        {
            QPainter painter(&icon);
            painter.setRenderHint(QPainter::SmoothPixmapTransform);
            painter.drawPixmap(QRect(2, 0, 10, 10), arrow);
        }
        setIcon(icon);
    }
};

class OptionsArea : public QScrollArea {
public:
    QSize sizeHint() const override {
        auto preferred = QScrollArea::sizeHint();
        preferred.setHeight(qMin(preferred.height(), 375));
        return preferred;
    }
};

QString expandPath(QString input) {
    static const QRegularExpression variable(R"(\$\{([A-Za-z_][A-Za-z_0-9]*)\})");
    const auto match = variable.match(input);
    if (match.hasMatch()) {
        const auto value = qEnvironmentVariable(match.captured(1).toLatin1().constData());
        if (value.isEmpty())
            return {};
        input.replace(match.capturedStart(), match.capturedLength(), value);
    }
    return QDir::toNativeSeparators(input);
}

QComboBox *displayResolutionPicker(const QString &fallback, QWidget *parent) {
    auto *picker = new QComboBox(parent);
    QSize current;
    QList<QSize> available;
#ifdef Q_OS_WIN
    DISPLAY_DEVICEW display{};
    display.cb = sizeof(display);
    for (DWORD index = 0; EnumDisplayDevicesW(nullptr, index, &display, 0); ++index) {
        if (display.StateFlags & DISPLAY_DEVICE_PRIMARY_DEVICE)
            break;
        display = {};
        display.cb = sizeof(display);
    }
    if (display.StateFlags & DISPLAY_DEVICE_PRIMARY_DEVICE) {
        DEVMODEW mode{};
        mode.dmSize = sizeof(mode);
        if (EnumDisplaySettingsW(display.DeviceName, ENUM_CURRENT_SETTINGS, &mode))
            current = QSize(mode.dmPelsWidth, mode.dmPelsHeight);
        for (DWORD index = 0; ; ++index) {
            mode = {};
            mode.dmSize = sizeof(mode);
            if (!EnumDisplaySettingsW(display.DeviceName, index, &mode))
                break;
            const QSize size(mode.dmPelsWidth, mode.dmPelsHeight);
            if (mode.dmBitsPerPel == 32 && size.width() >= 800 && size.height() >= 600
                && !available.contains(size))
                available.append(size);
        }
    }
#endif
    const auto label = [](QSize size) { return QStringLiteral("%1 × %2").arg(size.width()).arg(size.height()); };
    const auto value = [](QSize size) { return QStringLiteral("%1x%2").arg(size.width()).arg(size.height()); };
    if (current.width() >= 800 && current.height() >= 600) {
        picker->addItem(QObject::tr("Current display (%1)").arg(label(current)), value(current));
    } else {
        auto displayFallback = fallback;
        picker->addItem(QObject::tr("Default (%1)").arg(displayFallback.replace('x', QStringLiteral(" × "))), fallback);
    }
    std::sort(available.begin(), available.end(), [](QSize a, QSize b) {
        return a.width() == b.width() ? a.height() > b.height() : a.width() > b.width();
    });
    for (const auto size : available) {
        if (value(size) != picker->itemData(0).toString())
            picker->addItem(label(size), value(size));
    }
    return picker;
}

class Setup : public QDialog {
public:
    explicit Setup(QString program, QJsonObject manifest, const QDir &assets = QDir())
        : backend(std::move(program)), config(std::move(manifest)) {
        const auto product = config.value("product").toString();
        setWindowTitle(tr("%1 Setup").arg(product));
        setWindowFlag(Qt::WindowContextHelpButtonHint, false);
        setMinimumWidth(540);
        auto *outer = new QVBoxLayout(this);
        outer->setContentsMargins(0, 0, 0, 0);
        outer->setSpacing(0);
        const auto artwork = config.value("artwork").toString();
        if (!artwork.isEmpty())
            outer->addWidget(new Banner(product, assets.filePath(artwork)));
        auto *layout = new QVBoxLayout;
        layout->setContentsMargins(24, 20, 24, 20);
        layout->setSpacing(10);
        outer->addLayout(layout);

        status = new QLabel(tr("Checking your system…"));
        status->setWordWrap(true);
        status->setTextFormat(Qt::PlainText);
        layout->addWidget(status);
        bar = new QProgressBar;
        bar->setAccessibleName(tr("Installation progress"));
        bar->hide();
        layout->addWidget(bar);
        counter = new QLabel;
        counter->hide();
        layout->addWidget(counter);

        details = new QPlainTextEdit;
        details->setReadOnly(true);
        details->setMinimumHeight(details->fontMetrics().height() * 3);
        details->setMaximumBlockCount(200);
        details->setAccessibleName(tr("Installation details"));
        details->hide();
        layout->addWidget(details, 1);

        advanced = new DisclosureButton;
        advanced->setAccessibleName(tr("Advanced options"));
        advanced->setText(tr("Advanced options"));
        layout->addWidget(advanced, 0, Qt::AlignLeft);

        auto *panel = new QWidget;
        auto *form = new QFormLayout(panel);
        form->setContentsMargins(0, 0, 0, 4);
        form->setFormAlignment(Qt::AlignLeft | Qt::AlignTop);
        form->setFieldGrowthPolicy(QFormLayout::AllNonFixedFieldsGrow);
        for (const auto &value : config.value("paths").toArray()) {
            const auto info = value.toObject();
            const auto id = info.value("id").toString();
            const auto initial = expandPath(info.value("default").toString());
            const bool inspectOnChange = info.value("inspectOnChange").toBool();
            auto *edit = addPath(form, info.value("label").toString(), initial, info.value("browse").toString(), inspectOnChange);
            fields.append({id, info, edit});
            if (inspectOnChange)
                inspectedPaths.insert(id, edit->text().trimmed());
            connect(edit, &QLineEdit::textChanged, this, [this, id] {
                if (fieldToFix == id)
                    clearFieldError();
            });
        }
        const auto displayOption = config.value("displayResolution").toObject();
        resolutionFlag = displayOption.value("id").toString();
        if (!resolutionFlag.isEmpty()) {
            resolution = displayResolutionPicker(displayOption.value("fallback").toString(), panel);
            form->addRow(tr("&Game resolution"), resolution);
        }
        const auto options = config.value("options").toArray();
        if (!options.isEmpty()) {
            auto *list = new QWidget;
            auto *rows = new QVBoxLayout(list);
            for (const auto &value : options) {
                const auto info = value.toObject();
                const auto id = info.value("id").toString();
                const auto required = info.value("requires").toString();
                const auto group = info.value("group").toString();
                if (!group.isEmpty()) {
                    auto *caption = new QLabel(group);
                    auto f = caption->font();
                    f.setBold(true);
                    caption->setFont(f);
                    rows->addWidget(caption);
                }
                auto *box = new QCheckBox(info.value("label").toString());
                const bool defaultChecked = info.value("default").toBool();
                box->setChecked(defaultChecked);
                if (required.isEmpty()) {
                    rows->addWidget(box);
                } else {
                    auto *row = new QHBoxLayout;
                    row->addSpacing(style()->pixelMetric(QStyle::PM_IndicatorWidth));
                    row->addWidget(box);
                    rows->addLayout(row);
                }
                checkboxes.append({id, required, box, defaultChecked});
                connect(box, &QCheckBox::toggled, this, [this] { updateDependencies(); });
            }
            form->addRow(list);
            updateDependencies();
        }
        advancedPanel = new OptionsArea;
        advancedPanel->setAccessibleName(tr("Installation options"));
        advancedPanel->setFrameShape(QFrame::NoFrame);
        advancedPanel->setWidgetResizable(true);
        advancedPanel->setMinimumHeight(110);
        advancedPanel->setWidget(panel);
        advancedPanel->hide();
        layout->addWidget(advancedPanel, 1);
        connect(advanced, &QToolButton::toggled, this, [this](bool shown) {
            fitHeight();
            advancedPanel->setVisible(shown);
            if (reconfigurable && !running && !paused)
                primary->setText(shown ? tr("Apply changes") : tr("Configure"));
        });

        auto *actions = new QDialogButtonBox;
        removeInstalled = new QPushButton(tr("Remove mod…"));
        actions->addButton(removeInstalled, QDialogButtonBox::ActionRole);
        removeInstalled->hide();
        removeInstalled->setAutoDefault(false);
        connect(removeInstalled, &QPushButton::clicked, this, [this] {
            const auto answer = QMessageBox::question(this, tr("Remove %1").arg(config.value("product").toString()),
                tr("Remove the installed mod and restore the original game executable? The texture cache will be kept."),
                QMessageBox::Yes | QMessageBox::Cancel, QMessageBox::Cancel);
            if (answer == QMessageBox::Yes)
                startOperation("remove");
        });
        closeInstalled = actions->addButton(QDialogButtonBox::Close);
        closeInstalled->hide();
        closeInstalled->setAutoDefault(false);
        connect(closeInstalled, &QPushButton::clicked, this, [this] { close(); });
        pause = new QPushButton(tr("Pause"));
        pause->hide();
        actions->addButton(pause, QDialogButtonBox::RejectRole);
        pause->setAutoDefault(false);
        primary = new QPushButton(tr("Install"));
        actions->addButton(primary, QDialogButtonBox::AcceptRole);
        primary->setDefault(true);
        primary->setEnabled(false);
        layout->addStretch();
        layout->addWidget(actions);

        process.setProcessChannelMode(QProcess::SeparateChannels);
#ifdef Q_OS_WIN
        process.setCreateProcessArgumentsModifier([](QProcess::CreateProcessArguments *args) {
            args->flags |= 0x08000000; // CREATE_NO_WINDOW
        });
#endif
        connect(&process, &QProcess::readyReadStandardOutput, this, [this] { readEvents(); });
        connect(&process, &QProcess::readyReadStandardError, this, [this] { readDiagnostics(); });
        connect(&process, &QProcess::errorOccurred, this, [this](QProcess::ProcessError error) {
            logLine("process", process.errorString().toUtf8());
            if (error == QProcess::FailedToStart) {
                if (!inspecting || !inspectionOutdated())
                    showError(tr("Setup could not start. Try opening the installer again."));
                settle();
            }
        });
        connect(&process, &QProcess::finished, this, [this](int code, QProcess::ExitStatus exit) {
            readEvents();
            if (!pending.isEmpty()) {
                handleLine(pending);
                pending.clear();
            }
            readDiagnostics(true);
            logLine("process", QStringLiteral("Exit code %1 (%2)").arg(code)
                        .arg(exit == QProcess::NormalExit ? "normal exit" : "crash").toUtf8());
            const bool failed = code != 0 || exit != QProcess::NormalExit;
            if (!inspectionOutdated() && ((failed && completed) || ((running || failed) && !reported)))
                showError(tr("Setup stopped unexpectedly. Try again."));
            settle();
        });
        connect(primary, &QPushButton::clicked, this, [this] {
            if (running || inspecting)
                return;
            if (completed) {
                inspectSelection();
                return;
            }
            if (reconfigurable && !paused && !advanced->isChecked()) {
                advanced->setChecked(true);
                return;
            }
            if (installed && !reconfigurable) {
                if (launcher.isEmpty()) {
                    close();
                    return;
                }
                if (!QProcess::startDetached(launcher, {}, QFileInfo(launcher).absolutePath()))
                    showError(tr("Could not open %1.").arg(config.value("product").toString()));
                return;
            }
            for (const auto &field : fields) {
                if (field.required() && field.edit->text().trimmed().isEmpty()) {
                    showError(field.info.value("prompt").toString(tr("Choose the required folder.")), field.id);
                    return;
                }
            }
            startOperation(reconfigurable ? "configure" : "install");
        });
        connect(pause, &QPushButton::clicked, this, [this] { stop(); });
        resize(560, preferredHeight(560));
        startBackend("inspect");
    }

protected:
    void reject() override { close(); }

    void closeEvent(QCloseEvent *event) override {
        if (!running || process.state() == QProcess::NotRunning) {
            if (inspecting && process.state() != QProcess::NotRunning)
                process.kill();
            event->accept();
            return;
        }
        closeAfterStop = true;
        stop();
        event->ignore();
    }

    void resizeEvent(QResizeEvent *event) override {
        QDialog::resizeEvent(event);
        if (!fitPending && event->oldSize().isValid() &&
            (event->oldSize().height() != height() || advancedPanel->isHidden()))
            userHeight = height();
    }

private:
    int preferredHeight(int windowWidth) const {
        const int preferred = heightForWidth(windowWidth);
        return qMax(minimumSizeHint().height(), preferred >= 0 ? preferred : sizeHint().height());
    }

    void fitHeight() {
        if (fitPending)
            return;
        fitPending = true;
        QTimer::singleShot(0, this, [this] {
            const QRect available = screen()->availableGeometry();
            layout()->invalidate();
            layout()->activate();
            const int availableHeight = available.height() - (frameGeometry().height() - height());
            const int wantedHeight = userHeight > 0 && !advancedPanel->isHidden()
                ? userHeight : qMax(preferredHeight(width()), userHeight);
            const int minimumHeightForWidth = layout()->totalMinimumHeightForWidth(width());
            resize(width(), qMax(minimumHeightForWidth, qMin(wantedHeight, availableHeight)));
            if (isVisible()) {
                auto *window = windowHandle();
                const QRect frame = window->frameGeometry();
                const int top = qBound(available.top(), frame.top(),
                                       qMax(available.top(), available.bottom() - frame.height() + 1));
                if (top != frame.top())
                    window->setFramePosition(QPoint(frame.left(), top));
            }
            fitPending = false;
        });
    }

    struct Field {
        QString id;
        QJsonObject info;
        QLineEdit *edit;
        bool required() const { return info.value("required").toBool(); }
    };

    struct Option {
        QString id, required;
        QCheckBox *box;
        bool defaultChecked;
    };

    void updateDependencies() {
        // Parents precede their children in the manifest.
        QHash<QString, bool> active;
        for (const auto &option : checkboxes) {
            const bool enabled = option.required.isEmpty() || active.value(option.required);
            option.box->setEnabled(enabled);
            active.insert(option.id, enabled && option.box->isChecked());
        }
    }

    const Field *fieldById(const QString &id) const {
        for (const auto &field : fields)
            if (field.id == id)
                return &field;
        return nullptr;
    }

    void clearFieldError() {
        if (const auto *field = fieldById(fieldToFix)) {
            field->edit->setPalette(QPalette());
            field->edit->setAccessibleDescription({});
        }
        fieldToFix.clear();
    }

    QLineEdit *addPath(QFormLayout *layout, const QString &label, const QString &value, const QString &title, bool inspectOnChange) {
        auto *edit = new QLineEdit(value);
        edit->setAccessibleName(QString(label).remove('&'));
        auto *row = new QWidget;
        auto *items = new QHBoxLayout(row);
        items->setContentsMargins(0, 0, 0, 0);
        items->setSpacing(8);
        items->addWidget(edit);
        auto *button = new QPushButton(tr("Browse…"));
        button->setAccessibleName(title);
        items->addWidget(button);
        button->setAutoDefault(false);
        auto *caption = new QLabel(label);
        caption->setBuddy(edit);
        layout->addRow(caption, row);
        connect(button, &QPushButton::clicked, this, [this, edit, title, inspectOnChange] {
            if (browse(edit, title) && inspectOnChange)
                inspectSelection();
        });
        if (inspectOnChange)
            connect(edit, &QLineEdit::editingFinished, this, [this] {
                if (inspectionOutdated())
                    inspectSelection();
            });
        return edit;
    }

    bool inspectionOutdated() const {
        for (const auto &field : fields)
            if (field.info.value("inspectOnChange").toBool() &&
                field.edit->text().trimmed() != inspectedPaths.value(field.id))
                return true;
        return false;
    }

    void inspectSelection() {
        if (running || inspecting)
            return;
        fitHeight();
        inspecting = true;
        if (completed) {
            completed = false;
            details->clear();
            details->hide();
        }
        installed = reconfigurable = paused = reported = false;
        launcher.clear();
        clearFieldError();
        for (const auto &option : checkboxes)
            option.box->setChecked(option.defaultChecked);
        if (resolution)
            resolution->setCurrentIndex(0);
        // Reset paths loaded from a previous installation, while keeping user edits.
        for (const auto &field : fields) {
            if (installedPaths.contains(field.id) && field.edit->text() == installedPaths.value(field.id))
                field.edit->setText(expandPath(field.info.value("default").toString()));
        }
        installedPaths.clear();
        for (const auto &field : fields)
            if (field.info.value("inspectOnChange").toBool())
                inspectedPaths.insert(field.id, field.edit->text().trimmed());
        status->setText(tr("Checking the selected folder…"));
        primary->setEnabled(false);
        closeInstalled->hide();
        removeInstalled->hide();
        startBackend("inspect");
    }

    bool browse(QLineEdit *edit, const QString &title) {
        const auto selected = QFileDialog::getExistingDirectory(this, title, edit->text());
        if (selected.isEmpty())
            return false;
        edit->setText(QDir::toNativeSeparators(selected));
        return true;
    }

    QStringList arguments(const QString &command) const {
        QStringList args;
        for (const auto &arg : config.value("backendArgs").toArray())
            args << arg.toString();
        args << command;
        for (const auto &field : fields) {
            const auto value = field.edit->text().trimmed();
            if (!value.isEmpty())
                args << "-" + field.id << QDir::toNativeSeparators(value);
        }
        if (command == "install" || command == "configure") {
            if (resolution)
                args << "-" + resolutionFlag << resolution->currentData().toString();
            for (const auto &option : checkboxes)
                args << "-" + option.id + "=" + (option.box->isEnabled() && option.box->isChecked() ? "true" : "false");
        }
        return args;
    }

    void startOperation(const QString &command) {
        fitHeight();
        operation = command;
        running = true;
        paused = false;
        stopping = false;
        reported = false;
        retryRequested = false;
        const bool removing = operation == "remove";
        status->setText(removing ? tr("Removing the mod…") :
                        reconfigurable ? tr("Checking the installed mod…") : tr("Checking files…"));
        primary->hide();
        closeInstalled->hide();
        removeInstalled->hide();
        pause->setVisible(!removing);
        pause->setEnabled(true);
        if (!removing)
            pause->setFocus();
        advanced->setChecked(false);
        advanced->hide();
        details->clear();
        details->show();
        counter->hide();
        busyBar();
        startBackend(operation);
    }

    void stop() {
        if (!stopping) {
            stopping = true;
            pause->setEnabled(false);
            status->setText(config.value("pausingText").toString(tr("Pausing…")));
            logLine("setup", "Pause requested");
            appendDetails(tr("Pause requested.").toUtf8() + '\n');
            process.closeWriteChannel();
        }
    }

    static void logLine(const char *source, const QByteArray &message) {
        const auto entry = QDateTime::currentDateTimeUtc().toString(Qt::ISODateWithMs).toUtf8()
            + " [" + source + "] " + message + '\n';
        std::fwrite(entry.constData(), 1, entry.size(), stderr);
        std::fflush(stderr);
    }

    void startBackend(const QString &command) {
        const auto args = arguments(command);
        logLine("process", "Starting " + QJsonDocument(QJsonArray::fromStringList(args)).toJson(QJsonDocument::Compact));
        process.start(backend, args);
    }

    void readDiagnostics(bool finished = false) {
        diagnostics += process.readAllStandardError();
        qsizetype start = 0;
        for (qsizetype i = 0; i < diagnostics.size(); ++i) {
            if (diagnostics[i] == '\n' || diagnostics[i] == '\r') {
                logDiagnostic(diagnostics.mid(start, i - start));
                start = i + 1;
            }
        }
        diagnostics.remove(0, start);
        if (finished || diagnostics.size() > 16384) {
            logDiagnostic(diagnostics);
            diagnostics.clear();
        }
    }

    void logDiagnostic(const QByteArray &line) {
        if (line.isEmpty())
            return;
        logLine("backend", line);
        appendDetails(line + '\n');
    }

    void readEvents() {
        pending += process.readAllStandardOutput();
        qsizetype end;
        while ((end = pending.indexOf('\n')) >= 0) {
            handleLine(pending.first(end));
            pending.remove(0, end + 1);
        }
    }

    void handleLine(const QByteArray &line) {
        logLine("event", line);
        if (inspecting && inspectionOutdated())
            return;
        const auto event = QJsonDocument::fromJson(line).object();
        const auto type = event.value("type").toString();
        const auto text = event.value("text").toString();
        if (type == "field") {
            if (const auto *field = fieldById(event.value("field").toString())) {
                field->edit->setText(QDir::toNativeSeparators(event.value("path").toString()));
                if (inspecting && field->info.value("inspectOnChange").toBool())
                    inspectedPaths.insert(field->id, field->edit->text().trimmed());
                else if (!field->info.value("inspectOnChange").toBool())
                    installedPaths.insert(field->id, field->edit->text());
            }
        } else if (type == "step" && !stopping) {
            appendDetails(text.toUtf8() + '\n');
            status->setText(text);
            counter->hide();
            busyBar();
        } else if (type == "progress" && !stopping) {
            bar->setRange(0, 100);
            bar->setValue(event.value("value").toInt());
            bar->setTextVisible(true);
        } else if (type == "count" && !stopping) {
            const int done = event.value("value").toInt();
            const int total = event.value("total").toInt();
            if (total > 0) {
                bar->setRange(0, total);
                bar->setValue(done);
                bar->setTextVisible(true);
                counter->setText(tr("Resources processed: %L1 / %L2").arg(done).arg(total));
            } else {
                counter->setText(config.value("progressText").toString(tr("Completed: %L1")).arg(done));
            }
            counter->show();
        } else if (type == "installed") {
            if (running) {
                completed = true;
                const auto message = operation == "configure" ? tr("Changes applied.") : tr("Installation complete.");
                appendDetails(message.toUtf8() + '\n');
                status->setText(message);
            }
            launcher = event.value("path").toString();
            const auto settings = event.value("settings").toObject();
            reconfigurable = config.value("reconfigure").toBool() && !settings.isEmpty();
            if (reconfigurable) {
                if (!completed)
                    status->setText(config.value("installedText").toString(tr("Installation complete.")));
                const auto values = settings.value("options").toObject();
                for (const auto &option : checkboxes)
                    if (values.contains(option.id))
                        option.box->setChecked(values.value(option.id).toBool());
                const auto size = settings.value("resolution").toString();
                if (resolution && !size.isEmpty()) {
                    int index = resolution->findData(size);
                    if (index < 0) {
                        auto label = size;
                        resolution->addItem(label.replace('x', QStringLiteral(" × ")), size);
                        index = resolution->count() - 1;
                    }
                    resolution->setCurrentIndex(index);
                }
            }
            installed = true;
            reported = true;
        } else if (type == "removed") {
            installed = reconfigurable = false;
            reported = true;
            completed = true;
            appendDetails(tr("The mod was removed.").toUtf8() + '\n');
            status->setText(tr("The mod was removed."));
        } else if (type == "error") {
            showError(text, event.value("field").toString());
        } else if (type == "stopped") {
            appendDetails(tr("Installation paused.").toUtf8() + '\n');
            paused = true;
            reported = true;
            status->setText(config.value("pausedText").toString(tr("Resume whenever you’re ready.")));
            primary->setText(tr("Resume"));
        } else if (event.isEmpty()) {
            appendDetails(line + '\n');
        }
    }

    void appendDetails(const QByteArray &bytes) {
        if (bytes.isEmpty())
            return;
        auto *scroll = details->verticalScrollBar();
        const int previous = scroll->value();
        const bool follow = previous == scroll->maximum();
        auto cursor = QTextCursor(details->document());
        cursor.movePosition(QTextCursor::End);
        cursor.insertText(QString::fromUtf8(bytes).replace('\r', '\n'));
        scroll->setValue(follow ? scroll->maximum() : previous);
    }

    void showError(const QString &message, const QString &fieldId = {}) {
        fitHeight();
        logLine("error", message.toUtf8());
        appendDetails(message.toUtf8() + '\n');
        details->show();
        reported = true;
        paused = false;
        completed = false;
        if (!installed || reconfigurable) {
            status->setText(tr("Setup could not continue."));
            if (!reconfigurable)
                primary->setText(tr("Install"));
        }

        clearFieldError();
        fieldToFix = fieldId;
        const auto *field = fieldById(fieldToFix);
        if (field) {
            auto palette = field->edit->palette();
            const auto base = palette.color(QPalette::Base);
            palette.setColor(QPalette::Base, QColor((base.red() * 3 + 255) / 4, base.green() * 3 / 4, base.blue() * 3 / 4));
            field->edit->setPalette(palette);
            field->edit->setAccessibleDescription(message);
            advanced->show();
            advanced->setChecked(true);
        }

        const bool canRetry = running && fieldId.isEmpty();
        const auto buttons = canRetry ? QMessageBox::Retry | QMessageBox::Cancel : QMessageBox::StandardButtons(QMessageBox::Ok);
        auto *alert = new QMessageBox(QMessageBox::Critical, tr("Setup error"), message, buttons, this);
        alert->setTextFormat(Qt::PlainText);
        alert->setAttribute(Qt::WA_DeleteOnClose);
        if (field) {
            connect(alert, &QMessageBox::finished, this, [this, edit = field->edit] {
                advancedPanel->ensureWidgetVisible(edit);
                edit->setFocus();
                edit->selectAll();
            });
        } else if (canRetry) {
            connect(alert, &QMessageBox::finished, this, [this](int result) {
                if (result != QMessageBox::Retry)
                    return;
                if (running)
                    retryRequested = true;
                else
                    startOperation(operation);
            });
        }
        alert->open();
    }

    void busyBar() {
        bar->setRange(0, 0);
        bar->setTextVisible(false);
        bar->show();
    }

    void settle() {
        fitHeight();
        const bool wasInstalling = running;
        const bool inspectAgain = inspecting && inspectionOutdated();
        inspecting = false;
        running = false;
        stopping = false;
        if (inspectAgain) {
            inspectSelection();
            return;
        }
        bar->hide();
        pause->hide();
        primary->show();
        primary->setEnabled(true);
        if (completed) {
            counter->hide();
            advanced->setChecked(false);
            advanced->hide();
            removeInstalled->hide();
            closeInstalled->show();
            primary->setText(tr("Back to start"));
        } else if (reconfigurable) {
            counter->hide();
            advanced->setVisible(!fieldToFix.isEmpty());
            closeInstalled->show();
            removeInstalled->setVisible(!paused);
            if (!paused) {
                if (!reported)
                    status->setText(config.value("installedText").toString(tr("Installation complete.")));
                primary->setText(advanced->isChecked() ? tr("Apply changes") : tr("Configure"));
            }
        } else if (installed) {
            counter->hide();
            advanced->setChecked(false);
            advanced->hide();
            status->setText(config.value("installedText").toString(tr("Installation complete.")));
            primary->setText(launcher.isEmpty() ? tr("Close") : config.value("action").toString(tr("Open")));
        } else {
            primary->setText(paused ? tr("Resume") : tr("Install"));
            advanced->setVisible(!paused);
            if (!wasInstalling && !reported) {
                status->setText(config.value("readyText").toString(tr("Ready to install.")));
                for (const auto &field : fields) {
                    if (field.required() && field.edit->text().trimmed().isEmpty()) {
                        status->setText(field.info.value("prompt").toString(tr("Choose a folder to continue.")));
                        break;
                    }
                }
            }
            if (reported && !details->document()->isEmpty())
                details->show();
        }
        primary->setDefault(true);
        primary->setFocus();
        if (closeAfterStop)
            close();
        else if (retryRequested)
            QTimer::singleShot(0, this, [this] {
                if (isVisible() && retryRequested)
                    startOperation(operation);
            });
    }

    QString backend, launcher, fieldToFix, operation;
    QHash<QString, QString> inspectedPaths, installedPaths;
    QJsonObject config;
    QProcess process;
    QByteArray pending, diagnostics;
    int userHeight = 0;
    bool fitPending = false;
    bool installed = false, paused = false, completed = false;
    bool reconfigurable = false;
    bool running = false, inspecting = true, stopping = false, closeAfterStop = false, reported = false;
    bool retryRequested = false;
    QLabel *status, *counter;
    QProgressBar *bar;
    QToolButton *advanced;
    QScrollArea *advancedPanel;
    QPlainTextEdit *details;
    QList<Field> fields;
    QList<Option> checkboxes;
    QString resolutionFlag;
    QComboBox *resolution = nullptr;
    QPushButton *primary, *pause, *closeInstalled, *removeInstalled;
};

} // namespace

int main(int argc, char **argv) {
    QApplication app(argc, argv);
    const auto args = app.arguments();
    if (args.size() != 3)
        return 1;
    QFile file(args.at(2));
    QJsonObject config;
    if (file.open(QIODevice::ReadOnly))
        config = QJsonDocument::fromJson(file.readAll()).object();
    if (config.value("product").toString().isEmpty()) {
        QMessageBox::critical(nullptr, QObject::tr("Setup"), QObject::tr("The setup configuration is missing or invalid."));
        return 1;
    }
    const auto assets = QFileInfo(file).absoluteDir();
    const auto icon = config.value("icon").toString();
    if (!icon.isEmpty())
        app.setWindowIcon(QIcon(assets.filePath(icon)));
    Setup setup(args.at(1), config, assets);
    setup.show();
    return app.exec();
}
