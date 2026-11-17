#include <QtWidgets>
#include <QDesktopServices>
#include <QImageReader>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocale>
#include <algorithm>
#include <cstdio>
#include <utility>

#ifdef Q_OS_WIN
#include <qt_windows.h>
#elif defined(Q_OS_MACOS)
#include <CoreGraphics/CoreGraphics.h>
#endif

namespace {

QFile *sessionLog = nullptr;

static void logLine(const char *source, const QByteArray &message) {
    const auto entry = QDateTime::currentDateTimeUtc().toString(Qt::ISODateWithMs).toUtf8()
        + " [" + source + "] " + message + '\n';
    std::fwrite(entry.constData(), 1, entry.size(), stderr);
    std::fflush(stderr);
    if (sessionLog) {
        sessionLog->write(entry);
        sessionLog->flush();
    }
}

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

QJsonObject applyPlatformOverrides(QJsonObject config, const QString &platform) {
    const auto overrides = config.take("platforms").toObject().value(platform).toObject();
    for (auto it = overrides.constBegin(); it != overrides.constEnd(); ++it)
        config.insert(it.key(), it.value());
    return config;
}

QJsonObject resolveManifest(QJsonObject config) {
#ifdef Q_OS_WIN
    const auto platform = QStringLiteral("windows");
#elif defined(Q_OS_MACOS)
    const auto platform = QStringLiteral("macos");
#else
    const QString platform;
#endif
    config = applyPlatformOverrides(std::move(config), platform);
    for (const auto *key : {"paths", "options"}) {
        const auto items = config.value(key).toArray();
        QJsonArray resolved;
        for (const auto &value : items) {
            auto item = value.toObject();
            const auto target = item.value("platform").toString();
            if (target.isEmpty() || target == platform)
                resolved.append(applyPlatformOverrides(std::move(item), platform));
        }
        config.insert(key, resolved);
    }
    const auto resolution = config.value("displayResolution").toObject();
    config.insert("displayResolution", applyPlatformOverrides(resolution, platform));
    return config;
}

QString resolutionLabel(QString value) {
    return value == QStringLiteral("auto") ? QObject::tr("Automatic") : value.replace('x', QStringLiteral(" × "));
}

QComboBox *displayResolutionPicker(const QJsonObject &info, QWidget *parent) {
    auto *picker = new QComboBox(parent);
    const auto fallback = info.value("fallback").toString();
    const bool automatic = info.value("automatic").toBool();
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
#elif defined(Q_OS_MACOS)
    const auto pixelSize = [](CGDisplayModeRef mode) {
        const auto w = CGDisplayModeGetPixelWidth(mode);
        const auto h = CGDisplayModeGetPixelHeight(mode);
        if (w < 800 || w > 7680 || h < 600 || h > 4320)
            return QSize();
        return QSize(int(w), int(h));
    };
    const auto display = CGMainDisplayID();
    if (auto mode = CGDisplayCopyDisplayMode(display)) {
        current = pixelSize(mode);
        CFRelease(mode);
    }
    if (auto modes = CGDisplayCopyAllDisplayModes(display, nullptr)) {
        const auto count = CFArrayGetCount(modes);
        for (CFIndex index = 0; index < count; ++index) {
            const auto mode = static_cast<CGDisplayModeRef>(const_cast<void *>(CFArrayGetValueAtIndex(modes, index)));
            const QSize size = pixelSize(mode);
            if (size.isValid() && !available.contains(size))
                available.append(size);
        }
        CFRelease(modes);
    }
#endif
    const auto label = [](QSize size) { return QStringLiteral("%1 × %2").arg(size.width()).arg(size.height()); };
    const auto value = [](QSize size) { return QStringLiteral("%1x%2").arg(size.width()).arg(size.height()); };
    const bool hasCurrent = current.width() >= 800 && current.height() >= 600;
    if (fallback == QStringLiteral("auto")) {
        picker->addItem(QObject::tr("Automatic"), fallback);
        if (hasCurrent && !available.contains(current))
            available.append(current);
    } else if (hasCurrent) {
        picker->addItem(QObject::tr("Current display (%1)").arg(label(current)), value(current));
    } else {
        auto displayFallback = fallback;
        picker->addItem(QObject::tr("Default (%1)").arg(displayFallback.replace('x', QStringLiteral(" × "))), fallback);
    }
    if (automatic && fallback != QStringLiteral("auto"))
        picker->addItem(QObject::tr("Automatic"), QStringLiteral("auto"));
    std::sort(available.begin(), available.end(), [](QSize a, QSize b) {
        return a.width() == b.width() ? a.height() > b.height() : a.width() > b.width();
    });
    for (const auto size : available) {
        if (value(size) != picker->itemData(0).toString())
            picker->addItem(label(size), value(size));
    }
    for (const auto &entry : info.value("values").toArray()) {
        const auto size = entry.toString();
        if (!size.isEmpty() && picker->findData(size) < 0)
            picker->addItem(resolutionLabel(size), size);
    }
    picker->setEditable(info.value("editable").toBool());
    picker->setInsertPolicy(QComboBox::NoInsert);
    return picker;
}

QString defaultChoice(const QJsonObject &option, const QDir &assets, const QString &uiLanguage) {
    const auto fallback = option.value("default").toString();
    const auto language = uiLanguage.section('-', 0, 0).section('_', 0, 0).toLower();
    const auto localized = option.value("localeDefaults").toObject().value(language).toString();
    if (localized.isEmpty())
        return fallback;

    const auto files = option.value("localeDefaultFiles").toArray();
    auto hasFiles = [&files](const QDir &dir) {
        return std::all_of(files.begin(), files.end(), [&dir](const auto &file) {
            return QFileInfo(dir.filePath(file.toString())).isFile();
        });
    };
    const auto setupDir = qEnvironmentVariable("SETUP_EXE_DIR");
    if (hasFiles(assets) || (!setupDir.isEmpty() && hasFiles(QDir(setupDir))))
        return localized;
    return fallback;
}

class TextureTransferDialog : public QDialog {
public:
    struct Error {
        QString text, field;
    };

    TextureTransferDialog(bool exporting, QWidget *parent)
        : QDialog(parent), exporting(exporting) {
        setWindowTitle(exporting ? tr("Export textures") : tr("Import textures"));
        setWindowFlag(Qt::WindowContextHelpButtonHint, false);
        setMinimumWidth(420);

        auto *layout = new QVBoxLayout(this);
        status = new QLabel(tr("Checking the texture archive…"));
        status->setTextFormat(Qt::PlainText);
        status->setWordWrap(true);
        layout->addWidget(status);

        bar = new QProgressBar;
        bar->setAccessibleName(exporting ? tr("Packing progress") : tr("Unpacking progress"));
        bar->setRange(0, 0);
        layout->addWidget(bar);
        count = new QLabel;
        count->hide();
        layout->addWidget(count);

        auto *actions = new QDialogButtonBox;
        cancel = actions->addButton(QDialogButtonBox::Cancel);
        closeButton = actions->addButton(QDialogButtonBox::Close);
        closeButton->hide();
        layout->addWidget(actions);
        connect(cancel, &QPushButton::clicked, this, &TextureTransferDialog::reject);
        connect(closeButton, &QPushButton::clicked, this, &QDialog::accept);

#ifdef Q_OS_WIN
        process.setCreateProcessArgumentsModifier([](QProcess::CreateProcessArguments *args) {
            args->flags |= 0x08000000; // CREATE_NO_WINDOW
        });
#endif
        connect(&process, &QProcess::readyReadStandardOutput, this, [this] { readEvents(); });
        connect(&process, &QProcess::readyReadStandardError, this, [this] { readDiagnostics(); });
        connect(&process, &QProcess::errorOccurred, this, [this](QProcess::ProcessError error) {
            logLine("process", process.errorString().toUtf8());
            if (error == QProcess::FailedToStart)
                finish(tr("Could not start the texture transfer."));
        });
        connect(&process, &QProcess::finished, this, [this](int code, QProcess::ExitStatus exit) {
            readEvents();
            if (!pending.isEmpty())
                handleEvent(pending);
            readDiagnostics();
            logLine("process", QStringLiteral("Texture transfer exited with code %1").arg(code).toUtf8());
            if (!transferError.text.isEmpty()) {
                finish(transferError.text);
            } else if (cancelled && stopped) {
                QDialog::reject();
            } else if (transferred && code == 0 && exit == QProcess::NormalExit) {
                bar->setRange(0, 100);
                bar->setValue(100);
                finish(this->exporting ? tr("Textures exported.") : tr("Textures imported."), true);
            } else {
                finish(tr("The texture transfer stopped unexpectedly. See the setup log for details."));
            }
        });
    }

    Error run(const QString &backend, const QStringList &args) {
        logLine("process", "Starting " + QJsonDocument(QJsonArray::fromStringList(args)).toJson(QJsonDocument::Compact));
        process.start(backend, args);
        exec();
        return transferError;
    }

protected:
    void reject() override {
        if (process.state() == QProcess::NotRunning) {
            QDialog::reject();
        } else if (!cancelled) {
            cancelled = true;
            cancel->setEnabled(false);
            status->setText(tr("Cancelling…"));
            process.closeWriteChannel();
        }
    }

private:
    void readEvents() {
        pending += process.readAllStandardOutput();
        qsizetype end;
        while ((end = pending.indexOf('\n')) >= 0) {
            handleEvent(pending.first(end));
            pending.remove(0, end + 1);
        }
    }

    void readDiagnostics() {
        const auto message = process.readAllStandardError().trimmed();
        if (!message.isEmpty())
            logLine("backend", message);
    }

    void handleEvent(const QByteArray &line) {
        logLine("event", line);
        const auto event = QJsonDocument::fromJson(line).object();
        const auto type = event.value("type").toString();
        if (type == "textures-transferred") {
            transferred = true;
        } else if (type == "error") {
            transferError = {event.value("text").toString(), event.value("field").toString()};
        } else if (type == "stopped") {
            stopped = true;
        } else if (!cancelled && type == "step") {
            status->setText(exporting ? tr("Packing textures…") : tr("Unpacking textures…"));
        } else if (!cancelled && type == "count") {
            const int total = event.value("total").toInt();
            if (total > 0) {
                const int done = event.value("value").toInt();
                bar->setRange(0, total);
                bar->setValue(done);
                count->setText(tr("Files processed: %L1 / %L2").arg(done).arg(total));
                count->show();
            }
        }
    }

    void finish(const QString &message, bool successful = false) {
        status->setText(message);
        bar->setVisible(successful);
        count->setVisible(successful && !count->text().isEmpty());
        cancel->hide();
        closeButton->show();
        closeButton->setDefault(true);
        closeButton->setFocus();
    }

    QProcess process;
    QByteArray pending;
    Error transferError;
    QLabel *status, *count;
    QProgressBar *bar;
    QPushButton *cancel, *closeButton;
    const bool exporting;
    bool cancelled = false, stopped = false, transferred = false;
};

QStringList chooseExportGroups(QWidget *parent, const QString &backend, const QStringList &args) {
    QDialog dialog(parent);
    dialog.setWindowTitle(QObject::tr("Export textures"));
    dialog.setWindowFlag(Qt::WindowContextHelpButtonHint, false);
    dialog.setMinimumWidth(360);
    auto *layout = new QVBoxLayout(&dialog);
    auto *status = new QLabel(QObject::tr("Checking the texture cache…"));
    status->setTextFormat(Qt::PlainText);
    status->setWordWrap(true);
    layout->addWidget(status);
    auto *options = new QVBoxLayout;
    layout->addLayout(options);
    auto *buttons = new QDialogButtonBox(QDialogButtonBox::Ok | QDialogButtonBox::Cancel);
    auto *next = buttons->button(QDialogButtonBox::Ok);
    next->setText(QObject::tr("Choose archive…"));
    next->setEnabled(false);
    layout->addWidget(buttons);
    QObject::connect(buttons, &QDialogButtonBox::accepted, &dialog, &QDialog::accept);
    QObject::connect(buttons, &QDialogButtonBox::rejected, &dialog, &QDialog::reject);

    QProcess process;
#ifdef Q_OS_WIN
    process.setCreateProcessArgumentsModifier([](QProcess::CreateProcessArguments *args) {
        args->flags |= 0x08000000; // CREATE_NO_WINDOW
    });
#endif
    QList<QPair<QString, QCheckBox *>> choices;
    const auto update = [&] {
        next->setEnabled(std::any_of(choices.begin(), choices.end(), [](const auto &choice) {
            return choice.second->isChecked();
        }));
    };
    const auto scanError = QObject::tr("Could not read the texture cache.");
    QObject::connect(&process, &QProcess::errorOccurred, &dialog, [&](QProcess::ProcessError error) {
        logLine("process", process.errorString().toUtf8());
        if (error == QProcess::FailedToStart)
            status->setText(scanError);
    });
    QObject::connect(&process, &QProcess::finished, &dialog, [&](int code, QProcess::ExitStatus exit) {
        QJsonArray groups;
        QString message;
        for (const auto &line : process.readAllStandardOutput().split('\n')) {
            if (line.isEmpty())
                continue;
            logLine("event", line);
            const auto event = QJsonDocument::fromJson(line).object();
            if (event.value("type").toString() == "texture-groups")
                groups = event.value("groups").toArray();
            else if (event.value("type").toString() == "error")
                message = event.value("text").toString();
        }
        const auto diagnostics = process.readAllStandardError().trimmed();
        if (!diagnostics.isEmpty())
            logLine("backend", diagnostics);
        if (exit != QProcess::NormalExit || code != 0) {
            status->setText(message.isEmpty() ? scanError : message);
            return;
        }
        if (groups.isEmpty()) {
            status->setText(QObject::tr("This cache has no textures to export."));
            return;
        }
        status->setText(QObject::tr("Choose what to include in the archive."));
        for (const auto &value : groups) {
            const auto group = value.toObject();
            auto *box = new QCheckBox(group.value("label").toString());
            box->setChecked(group.value("prepared").toBool());
            choices.append({group.value("id").toString(), box});
            options->addWidget(box);
            QObject::connect(box, &QCheckBox::toggled, &dialog, update);
        }
        update();
    });
    logLine("process", "Starting " + QJsonDocument(QJsonArray::fromStringList(args)).toJson(QJsonDocument::Compact));
    process.start(backend, args);
    const auto result = dialog.exec();
    if (process.state() != QProcess::NotRunning) {
        process.kill();
        process.waitForFinished();
    }
    QStringList selected;
    if (result == QDialog::Accepted) {
        for (const auto &choice : choices)
            if (choice.second->isChecked())
                selected << choice.first;
    }
    return selected;
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

        overallBar = new QProgressBar;
        overallBar->setAccessibleName(tr("Overall installation progress"));
        overallBar->setFormat(tr("%v of %m stages complete"));
        overallBar->hide();
        layout->addWidget(overallBar);

        status = new QLabel(tr("Checking your system…"));
        status->setWordWrap(true);
        status->setTextFormat(Qt::PlainText);
        layout->addWidget(status);
        exportTextures = new QPushButton(tr("Export textures…"));
        exportTextures->setAutoDefault(false);
        exportTextures->hide();
        connect(exportTextures, &QPushButton::clicked, this, [this] { chooseTextureArchive(true); });
        bar = new QProgressBar;
        bar->setAccessibleName(tr("Current stage progress"));
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
            auto *edit = addPath(form, info, initial);
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
            resolution = displayResolutionPicker(displayOption, panel);
            form->addRow(displayOption.value("label").toString(tr("&Game resolution")), resolution);
            connect(resolution, &QComboBox::currentTextChanged, this, [this] {
                if (fieldToFix == resolutionFlag)
                    clearFieldError();
            });
        }
        const auto options = config.value("options").toArray();
        if (!options.isEmpty()) {
            auto *list = new QWidget;
            auto *rows = new QVBoxLayout(list);
            const auto uiLanguage = QLocale::system().uiLanguages().value(0);
            for (const auto &value : options) {
                const auto info = value.toObject();
                const auto id = info.value("id").toString();
                const auto backendId = info.value("backendId").toString(id);
                const auto required = info.value("requires").toString();
                const auto target = info.value("target").toString();
                const auto group = info.value("group").toString();
                if (!group.isEmpty()) {
                    auto *caption = new QLabel(group);
                    auto f = caption->font();
                    f.setBold(true);
                    caption->setFont(f);
                    rows->addWidget(caption);
                }
                const auto label = info.value("label").toString();
                const auto values = info.value("values").toArray();
                if (!values.isEmpty()) {
                    auto *picker = new QComboBox;
                    for (const auto &value : values) {
                        const auto item = value.toObject();
                        picker->addItem(item.value("label").toString(), item.value("value").toString());
                    }
                    const int defaultIndex = qMax(0, picker->findData(defaultChoice(info, assets, uiLanguage)));
                    picker->setCurrentIndex(defaultIndex);
                    picker->setAccessibleName(label);
                    auto *wrapper = new QWidget;
                    auto *row = new QHBoxLayout(wrapper);
                    row->setContentsMargins(0, 0, 0, 0);
                    if (!required.isEmpty())
                        row->addSpacing(style()->pixelMetric(QStyle::PM_IndicatorWidth));
                    auto *caption = new QLabel(label);
                    caption->setBuddy(picker);
                    row->addWidget(caption);
                    row->addWidget(picker);
                    row->addStretch();
                    rows->addWidget(wrapper);
                    choices.append({backendId, required, target, picker, caption, wrapper, defaultIndex});
                    continue;
                }
                auto *box = new QCheckBox(label);
                const bool defaultChecked = info.value("default").toBool();
                box->setChecked(defaultChecked);
                QWidget *wrapper = box;
                if (required.isEmpty()) {
                    rows->addWidget(box);
                } else {
                    wrapper = new QWidget;
                    auto *row = new QHBoxLayout(wrapper);
                    row->setContentsMargins(0, 0, 0, 0);
                    row->addSpacing(style()->pixelMetric(QStyle::PM_IndicatorWidth));
                    row->addWidget(box);
                    rows->addWidget(wrapper);
                }
                checkboxes.append({id, backendId, required, target, label,
                                   info.value("targetLabels").toObject(), box, wrapper, defaultChecked});
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

        layout->addStretch();
        auto *footer = new QHBoxLayout;
        const QUrl projectUrl(config.value("projectUrl").toString());
        if (projectUrl.isValid() && projectUrl.scheme() == "https" && !projectUrl.host().isEmpty()) {
            const auto label = config.value("projectLinkText").toString(tr("Project website"));
            auto *link = new QLabel;
            link->setTextFormat(Qt::RichText);
            link->setTextInteractionFlags(Qt::LinksAccessibleByMouse | Qt::LinksAccessibleByKeyboard);
            link->setOpenExternalLinks(true);
            link->setToolTip(projectUrl.toDisplayString());
            link->setText(QStringLiteral("<a href=\"%1\">%2</a>")
                .arg(QString::fromUtf8(projectUrl.toEncoded()).toHtmlEscaped(), label.toHtmlEscaped()));
            footer->addWidget(link);
        }
        footer->addWidget(exportTextures);
        moreActions = new QPushButton(tr("More actions"));
        moreActions->setAutoDefault(false);
        moreActions->hide();
        auto *actions = new QMenu(moreActions);
        moreActions->setMenu(actions);
        importTextures = actions->addAction(tr("Import textures…"));
        connect(importTextures, &QAction::triggered, this, [this] { chooseTextureArchive(false); });
        removeInstalled = actions->addAction(tr("Remove mod…"));
        footer->addWidget(moreActions);
        connect(removeInstalled, &QAction::triggered, this, [this] {
            const auto answer = QMessageBox::question(this, tr("Remove %1").arg(config.value("product").toString()),
                tr("Remove the installed mod and restore the original game executable? The texture cache will be kept."),
                QMessageBox::Yes | QMessageBox::Cancel, QMessageBox::Cancel);
            if (answer == QMessageBox::Yes)
                startOperation("remove");
        });

        footer->addStretch();
        closeInstalled = new QPushButton(tr("Close"));
        closeInstalled->hide();
        closeInstalled->setAutoDefault(false);
        footer->addWidget(closeInstalled);
        connect(closeInstalled, &QPushButton::clicked, this, [this] { close(); });
        primary = new QPushButton(tr("Install"));
        footer->addWidget(primary);
        pause = new QPushButton(tr("Pause"));
        pause->hide();
        footer->addWidget(pause);
        pause->setAutoDefault(false);
        const QUrl completionUrl(config.value("completionUrl").toString());
        play = new QPushButton(config.value("completionAction").toString(tr("Open")));
        footer->addWidget(play);
        play->hide();
        play->setAutoDefault(false);
        connect(play, &QPushButton::clicked, this, [this, completionUrl] {
            const auto url = handoffFolder.isEmpty() ? completionUrl : QUrl::fromLocalFile(handoffFolder);
            if (!QDesktopServices::openUrl(url))
                QMessageBox::warning(this, tr("Could not launch"), tr("Could not open %1.")
                    .arg(config.value("product").toString()));
        });
        primary->setDefault(true);
        primary->setEnabled(false);
        layout->addLayout(footer);

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
            const bool refresh = std::exchange(recovered, false) && !failed;
            if (!inspectionOutdated() && ((failed && completed) || ((running || failed) && !reported)))
                showError(tr("Setup stopped unexpectedly. Try again."));
            settle();
            if (refresh && !closeAfterStop)
                inspectSelection();
        });
        connect(primary, &QPushButton::clicked, this, [this] {
            if (running || inspecting)
                return;
            if (completed) {
                inspectSelection();
                return;
            }
            if (inspectionOutdated()) {
                inspectSelection();
                return;
            }
            if (paused) {
                startOperation(operation);
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
        if (config.value("checkForUpdates").toBool() && projectUrl.isValid() &&
            projectUrl.scheme() == "https" && !projectUrl.host().isEmpty())
            checkForUpdates(projectUrl);
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
    void checkForUpdates(QUrl projectUrl) {
        auto *check = new QProcess(this);
#ifdef Q_OS_WIN
        check->setCreateProcessArgumentsModifier([](QProcess::CreateProcessArguments *args) {
            args->flags |= 0x08000000; // CREATE_NO_WINDOW
        });
#endif
        connect(check, &QProcess::errorOccurred, check, [check](QProcess::ProcessError error) {
            if (error == QProcess::FailedToStart)
                check->deleteLater();
        });
        connect(check, &QProcess::finished, this, [this, check, projectUrl](int code, QProcess::ExitStatus exit) {
            const auto version = QString::fromUtf8(check->readAllStandardOutput()).trimmed();
            check->deleteLater();
            if (code != 0 || exit != QProcess::NormalExit || version.isEmpty() || !isVisible())
                return;
            auto release = projectUrl;
            release.setPath(projectUrl.path() + "/releases/tag/" + version);
            auto *notice = new QMessageBox(QMessageBox::Information, tr("Update available"),
                tr("%1 Setup %2 is available.").arg(config.value("product").toString(), version), QMessageBox::Cancel, this);
            notice->setTextFormat(Qt::PlainText);
            notice->setAttribute(Qt::WA_DeleteOnClose);
            auto *open = notice->addButton(tr("Open GitHub Releases"), QMessageBox::AcceptRole);
            connect(open, &QPushButton::clicked, notice, [release] { QDesktopServices::openUrl(release); });
            notice->setDefaultButton(QMessageBox::Cancel);
            notice->open();
        });
        check->start(backend, backendCommand("check-update"));
    }

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
        QString id, backendId, required, target, label;
        QJsonObject targetLabels;
        QCheckBox *box;
        QWidget *row;
        bool defaultChecked;
    };

    struct Choice {
        QString backendId, required, target;
        QComboBox *picker;
        QLabel *caption;
        QWidget *row;
        int defaultIndex;
    };

    bool supportsTarget(const QString &target) const {
        return target.isEmpty() || target == selectedTarget;
    }

    void updateDependencies() {
        // Parents precede their children in the manifest.
        QHash<QString, bool> active;
        for (const auto &option : checkboxes) {
            const bool shown = supportsTarget(option.target);
            const bool enabled = shown && (option.required.isEmpty() || active.value(option.required));
            option.row->setVisible(shown);
            option.box->setText(option.targetLabels.value(selectedTarget).toString(option.label));
            option.box->setEnabled(enabled);
            active.insert(option.id, enabled && option.box->isChecked());
        }
        for (const auto &choice : choices) {
            const bool shown = supportsTarget(choice.target);
            const bool enabled = shown && (choice.required.isEmpty() || active.value(choice.required));
            choice.row->setVisible(shown);
            choice.picker->setEnabled(enabled);
            choice.caption->setEnabled(enabled);
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
        if (resolution && fieldToFix == resolutionFlag)
            resolution->setAccessibleDescription({});
        fieldToFix.clear();
    }

    QWidget *markFieldError(const QString &message, const QString &fieldId) {
        clearFieldError();
        fieldToFix = fieldId;
        QWidget *input = nullptr;
        if (const auto *field = fieldById(fieldId)) {
            auto palette = field->edit->palette();
            const auto base = palette.color(QPalette::Base);
            palette.setColor(QPalette::Base, QColor((base.red() * 3 + 255) / 4, base.green() * 3 / 4, base.blue() * 3 / 4));
            field->edit->setPalette(palette);
            input = field->edit;
        } else if (resolution && fieldId == resolutionFlag) {
            input = resolution;
        }
        if (input) {
            input->setAccessibleDescription(message);
            advanced->show();
            advanced->setChecked(true);
        }
        return input;
    }

    void focusField(QWidget *input) {
        advancedPanel->ensureWidgetVisible(input);
        input->setFocus();
        if (auto *edit = qobject_cast<QLineEdit *>(input))
            edit->selectAll();
    }

    QLineEdit *addPath(QFormLayout *layout, const QJsonObject &info, const QString &value) {
        const auto label = info.value("label").toString();
        const auto title = info.value("browse").toString();
        const auto filter = info.value("fileFilter").toString();
        const bool inspectOnChange = info.value("inspectOnChange").toBool();
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
        const auto choose = [this, edit, title, inspectOnChange](const QString &fileFilter) {
            const auto path = fileFilter.isEmpty()
                ? QFileDialog::getExistingDirectory(this, title, edit->text())
                : QFileDialog::getOpenFileName(this, title, edit->text(), fileFilter);
            if (path.isEmpty())
                return;
            edit->setText(QDir::toNativeSeparators(path));
            if (inspectOnChange)
                inspectSelection();
        };
        if (filter.isEmpty()) {
            connect(button, &QPushButton::clicked, this, [choose] { choose({}); });
        } else {
            auto *menu = new QMenu(button);
            menu->addAction(tr("Choose folder…"), this, [choose] { choose({}); });
            menu->addAction(tr("Choose file…"), this, [choose, filter] { choose(filter); });
            button->setMenu(menu);
        }
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
        completed = false;
        details->clear();
        details->hide();
        counter->hide();
        overallBar->hide();
        installed = reconfigurable = paused = reported = false;
        launcher.clear();
        readyText.clear();
        handoffFolder.clear();
        selectedTarget.clear();
        clearFieldError();
        for (const auto &option : checkboxes)
            option.box->setChecked(option.defaultChecked);
        for (const auto &choice : choices)
            choice.picker->setCurrentIndex(choice.defaultIndex);
        if (resolution)
            selectResolution(resolution->itemData(0).toString());
        updateDependencies();
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
        play->hide();
        moreActions->hide();
        exportTextures->hide();
        startBackend("inspect");
    }

    bool canExportTextures() const {
        return config.value("textureArchives").toBool() && completed && (installed || !handoffFolder.isEmpty());
    }

    void chooseTextureArchive(bool exporting) {
        if (running || inspecting || paused || (exporting ? !canExportTextures() : completed))
            return;
        QStringList cacheArgs;
        if (const auto *cache = fieldById("cache")) {
            const auto folder = cache->edit->text().trimmed();
            if (!folder.isEmpty())
                cacheArgs << "-cache" << QDir::toNativeSeparators(folder);
        }
        QStringList groups;
        if (exporting && config.value("textureExportSelection").toBool()) {
            groups = chooseExportGroups(this, backend, backendCommand("list-textures") + cacheArgs);
            if (groups.isEmpty())
                return;
        }
        const auto filter = tr("Texture archives (*.zip)");
        auto path = exporting
            ? QFileDialog::getSaveFileName(this, tr("Export textures"),
                  QDir::home().filePath("SimCity4UHD-Textures.zip"), filter, nullptr, QFileDialog::DontConfirmOverwrite)
            : QFileDialog::getOpenFileName(this, tr("Import textures"), QDir::homePath(), filter);
        if (path.isEmpty())
            return;
        if (exporting && QFileInfo(path).suffix().isEmpty())
            path += QStringLiteral(".zip");
        if (exporting && QFileInfo::exists(path)) {
            QMessageBox::warning(this, tr("Export textures"), tr("Choose a new file name for the archive."));
            return;
        }
        auto args = backendCommand(exporting ? "export-textures" : "import-textures") + cacheArgs;
        if (!groups.isEmpty())
            args << "-groups" << groups.join(',');
        if (!exporting && config.value("detectTarget").toBool()) {
            if (const auto *game = fieldById("game")) {
                const auto path = game->edit->text().trimmed();
                if (!path.isEmpty())
                    args << "-game" << QDir::toNativeSeparators(path);
            }
        }
        args << "-archive" << QDir::toNativeSeparators(path);
        TextureTransferDialog dialog(exporting, this);
        const auto error = dialog.run(backend, args);
        if (!error.field.isEmpty()) {
            if (auto *input = markFieldError(error.text, error.field))
                focusField(input);
        }
    }

    void selectResolution(const QString &size) {
        if (!resolution || size.isEmpty())
            return;
        auto index = resolution->findData(size);
        if (index < 0) {
            resolution->addItem(resolutionLabel(size), size);
            index = resolution->count() - 1;
        }
        resolution->setCurrentIndex(index);
        if (resolution->isEditable())
            resolution->setEditText(resolution->itemText(index));
    }

    QString resolutionValue() const {
        const auto text = resolution->currentText().trimmed();
        const auto index = resolution->findText(text);
        if (index >= 0)
            return resolution->itemData(index).toString();
        return QString(text).toLower().remove(' ').replace(QChar(0x00d7), QLatin1Char('x'));
    }

    QStringList backendCommand(const QString &command) const {
        QStringList args;
        for (const auto &arg : config.value("backendArgs").toArray())
            args << arg.toString();
        args << command;
        return args;
    }

    QStringList arguments(const QString &command) const {
        auto args = backendCommand(command);
        if (command != "inspect" && config.value("detectTarget").toBool())
            args << "-target=" + selectedTarget;
        for (const auto &field : fields) {
            const auto value = field.edit->text().trimmed();
            if (!value.isEmpty())
                args << "-" + field.id << QDir::toNativeSeparators(value);
        }
        if (command == "install" || command == "configure") {
            if (resolution)
                args << "-" + resolutionFlag << resolutionValue();
            for (const auto &option : checkboxes)
                if (supportsTarget(option.target))
                    args << "-" + option.backendId + "=" + (option.box->isEnabled() && option.box->isChecked() ? "true" : "false");
            for (const auto &choice : choices)
                if (choice.picker->isEnabled())
                    args << "-" + choice.backendId + "=" + choice.picker->currentData().toString();
        }
        return args;
    }

    void startOperation(const QString &command) {
        if (inspectionOutdated() || (config.value("detectTarget").toBool() && selectedTarget.isEmpty())) {
            inspectSelection();
            return;
        }
        if ((command == "install" || command == "configure") && resolution && resolutionValue().isEmpty()) {
            showError(tr("Choose Automatic or enter a game resolution."), resolutionFlag);
            return;
        }
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
        moreActions->hide();
        exportTextures->hide();
        pause->setVisible(!removing);
        pause->setEnabled(true);
        if (!removing)
            pause->setFocus();
        advanced->setChecked(false);
        advanced->hide();
        details->clear();
        details->show();
        counter->hide();
        overallBar->hide();
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
        } else if (type == "overall" && running && !stopping) {
            const int total = event.value("total").toInt();
            const int done = event.value("value").toInt(-1);
            if (total > 0 && done >= 0 && done <= total) {
                if (overallBar->isHidden())
                    fitHeight();
                overallBar->setRange(0, total);
                overallBar->setValue(done);
                overallBar->show();
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
        } else if (type == "target" && inspecting) {
            selectedTarget = event.value("target").toString();
            updateDependencies();
            fitHeight();
        } else if (type == "ready" && inspecting) {
            readyText = text;
            selectResolution(event.value("settings").toObject().value("resolution").toString());
        } else if (type == "handoff" && running) {
            completed = reported = true;
            handoffFolder = event.value("path").toString();
            status->setText(text);
        } else if (type == "installed") {
            if (running) {
                completed = true;
                const auto message = operation == "configure" ? tr("Changes applied.") : tr("Installation complete.");
                status->setText(message);
            }
            launcher = event.value("path").toString();
            const auto settings = event.value("settings").toObject();
            reconfigurable = config.value("reconfigure").toBool() && !settings.isEmpty();
            if (reconfigurable) {
                if (!completed)
                    status->setText(text.isEmpty() ? config.value("installedText").toString(tr("Installation complete.")) : text);
                const auto values = settings.value("options").toObject();
                for (const auto &option : checkboxes)
                    if (values.contains(option.backendId))
                        option.box->setChecked(values.value(option.backendId).toBool());
                const auto selections = settings.value("choices").toObject();
                for (const auto &choice : choices) {
                    const int index = choice.picker->findData(selections.value(choice.backendId).toString());
                    if (index >= 0)
                        choice.picker->setCurrentIndex(index);
                }
                selectResolution(settings.value("resolution").toString());
            }
            installed = true;
            reported = true;
        } else if (type == "removed") {
            installed = reconfigurable = false;
            reported = true;
            completed = true;
            status->setText(tr("The mod was removed."));
        } else if (type == "recovered" && running) {
            recovered = true;
            reported = true;
            completed = true;
        } else if (type == "retry-resource" && running && !stopping) {
            askResourceRetry(text);
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

    void askResourceRetry(const QString &message) {
        bar->hide();
        counter->hide();
        status->setText(tr("Download interrupted."));
        auto *alert = new QMessageBox(QMessageBox::Warning, tr("Download failed"), message,
            QMessageBox::Retry | QMessageBox::Cancel, this);
        alert->setTextFormat(Qt::PlainText);
        alert->setAttribute(Qt::WA_DeleteOnClose);
        resourceRetry = alert;
        connect(alert, &QMessageBox::finished, this, [this](int answer) {
            resourceRetry.clear();
            if (!running || stopping)
                return;
            if (answer == QMessageBox::Retry)
                process.write("retry\n");
            else
                stop();
        });
        alert->open();
    }

    void showError(const QString &message, const QString &fieldId = {}) {
        fitHeight();
        logLine("error", message.toUtf8());
        reported = true;
        paused = false;
        completed = false;
        counter->hide();
        if (!installed || reconfigurable) {
            status->setText(tr("Setup could not continue."));
            if (!reconfigurable)
                primary->setText(tr("Install"));
        }

        auto *input = markFieldError(message, fieldId);
        const bool showDetails = !inspecting && !input;
        if (showDetails)
            appendDetails(message.toUtf8() + '\n');
        details->setVisible(showDetails);
        const bool canRetry = running && fieldId.isEmpty();
        const auto buttons = canRetry ? QMessageBox::Retry | QMessageBox::Cancel : QMessageBox::StandardButtons(QMessageBox::Ok);
        auto *alert = new QMessageBox(QMessageBox::Critical, tr("Setup error"), message, buttons, this);
        alert->setTextFormat(Qt::PlainText);
        alert->setAttribute(Qt::WA_DeleteOnClose);
        if (input) {
            connect(alert, &QMessageBox::finished, this, [this, input] { focusField(input); });
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
        if (resourceRetry)
            resourceRetry->close();
        if (inspectAgain) {
            inspectSelection();
            return;
        }
        bar->hide();
        if (!paused)
            overallBar->hide();
        pause->hide();
        primary->show();
        primary->setEnabled(true);
        play->setText(handoffFolder.isEmpty() ? config.value("completionAction").toString(tr("Open")) : tr("Open folder"));
        play->setVisible(completed && (!handoffFolder.isEmpty() || (installed && !config.value("completionUrl").toString().isEmpty())));
        const bool archives = config.value("textureArchives").toBool();
        importTextures->setVisible(archives);
        removeInstalled->setVisible(reconfigurable);
        moreActions->setVisible(!completed && !paused && (archives || reconfigurable));
        exportTextures->setVisible(canExportTextures());
        if (completed) {
            counter->hide();
            details->hide();
            advanced->setChecked(false);
            advanced->hide();
            closeInstalled->show();
            primary->setText(tr("Back to start"));
        } else if (reconfigurable) {
            counter->hide();
            advanced->setVisible(!fieldToFix.isEmpty());
            closeInstalled->show();
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
                status->setText(readyText.isEmpty() ? config.value("readyText").toString(tr("Ready to install.")) : readyText);
                for (const auto &field : fields) {
                    if (field.required() && field.edit->text().trimmed().isEmpty()) {
                        status->setText(field.info.value("prompt").toString(tr("Choose a folder to continue.")));
                        break;
                    }
                }
            }
        }
        auto *defaultAction = play->isVisibleTo(this) ? play : primary;
        defaultAction->setDefault(true);
        defaultAction->setFocus();
        if (closeAfterStop)
            close();
        else if (retryRequested)
            QTimer::singleShot(0, this, [this] {
                if (isVisible() && retryRequested)
                    startOperation(operation);
            });
    }

    QString backend, launcher, fieldToFix, operation;
    QString readyText, handoffFolder, selectedTarget;
    QHash<QString, QString> inspectedPaths, installedPaths;
    QJsonObject config;
    QProcess process;
    QPointer<QMessageBox> resourceRetry;
    QByteArray pending, diagnostics;
    int userHeight = 0;
    bool fitPending = false;
    bool installed = false, paused = false, completed = false;
    bool reconfigurable = false;
    bool running = false, inspecting = true, stopping = false, closeAfterStop = false, reported = false;
    bool retryRequested = false, recovered = false;
    QLabel *status, *counter;
    QProgressBar *overallBar, *bar;
    QToolButton *advanced;
    QScrollArea *advancedPanel;
    QPlainTextEdit *details;
    QList<Field> fields;
    QList<Option> checkboxes;
    QList<Choice> choices;
    QString resolutionFlag;
    QComboBox *resolution = nullptr;
    QPushButton *primary, *pause, *closeInstalled, *play, *moreActions, *exportTextures;
    QAction *importTextures, *removeInstalled;
};

} // namespace

int main(int argc, char **argv) {
    QApplication app(argc, argv);
    auto args = app.arguments();
#ifdef Q_OS_MACOS
    if (args.size() == 1) {
        const QDir bin(QCoreApplication::applicationDirPath());
        qputenv("SETUP_EXE_DIR", QFile::encodeName(QDir::cleanPath(bin.filePath("../../.."))));
        args << bin.filePath("simcity4-uhd") << bin.filePath("../Resources/setup.json");
    }
#endif
    if (args.size() != 3)
        return 1;
    QFile file(args.at(2));
    QJsonObject config;
    if (file.open(QIODevice::ReadOnly))
        config = resolveManifest(QJsonDocument::fromJson(file.readAll()).object());
    if (config.value("product").toString().isEmpty()) {
        QMessageBox::critical(nullptr, QObject::tr("Setup"), QObject::tr("The setup configuration is missing or invalid."));
        return 1;
    }
    QTemporaryFile log;
    const auto logDirectory = expandPath(config.value("sessionLogDirectory").toString());
    if (QDir::isAbsolutePath(logDirectory) && QDir().mkpath(logDirectory)) {
        log.setFileTemplate(QDir(logDirectory).filePath("setup-XXXXXX.log"));
        if (log.open()) {
            log.setAutoRemove(false);
            sessionLog = &log;
            logLine("setup", "Setup started (timestamps in UTC)");
        }
    }
    const auto assets = QFileInfo(file).absoluteDir();
    const auto icon = config.value("icon").toString();
    if (!icon.isEmpty())
        app.setWindowIcon(QIcon(assets.filePath(icon)));
    Setup setup(args.at(1), config, assets);
    setup.show();
    const auto result = app.exec();
    if (sessionLog) {
        logLine("setup", "Setup closed");
        sessionLog = nullptr;
    }
    return result;
}
